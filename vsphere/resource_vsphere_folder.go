package vsphere

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func resourceVSphereFolder() *schema.Resource {
	return &schema.Resource{
		Create: resourceVSphereFolderCreate,
		Read:   resourceVSphereFolderRead,
		Update: resourceVSphereFolderUpdate,
		Delete: resourceVSphereFolderDelete,
		Importer: &schema.ResourceImporter{
			State: resourceVSphereFolderImport,
		},

		SchemaVersion: 1,
		MigrateState:  resourceVSphereFolderMigrateState,
		Schema: map[string]*schema.Schema{
			"path": {
				Type:         schema.TypeString,
				Description:  "The path of the folder and any parents, relative to the datacenter and folder type being defined.",
				Required:     true,
				StateFunc:    normalizeFolderPath,
				ValidateFunc: validation.NoZeroValues,
			},
			"type": {
				Type:        schema.TypeString,
				Description: "The type of the folder.",
				ForceNew:    true,
				Required:    true,
				ValidateFunc: validation.StringInSlice(
					[]string{
						string(vSphereFolderTypeVM),
						string(vSphereFolderTypeNetwork),
						string(vSphereFolderTypeHost),
						string(vSphereFolderTypeDatastore),
						string(vSphereFolderTypeDatacenter),
					},
					false,
				),
			},
			"datacenter_id": {
				Type:        schema.TypeString,
				Description: "The ID of the datacenter. Can be ignored if creating a datacenter folder, otherwise required.",
				ForceNew:    true,
				Optional:    true,
			},
			// Tagging
			vSphereTagAttributeKey: tagsSchema(),
		},
	}
}

func resourceVSphereFolderCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	tagsClient, err := tagsClientIfDefined(d, meta)
	if err != nil {
		return err
	}

	ft := vSphereFolderType(d.Get("type").(string))
	var dc *object.Datacenter
	if dcID, ok := d.GetOk("datacenter_id"); ok {
		var err error
		dc, err = datacenterFromID(client, dcID.(string))
		if err != nil {
			return fmt.Errorf("cannot locate datacenter: %s", err)
		}
	} else {
		if ft != vSphereFolderTypeDatacenter {
			return fmt.Errorf("datacenter_id cannot be empty when creating a folder of type %s", ft)
		}
	}

	p := d.Get("path").(string)

	// Determine the parent folder
	parent, err := parentFolderFromPath(client, p, ft, dc)
	if err != nil {
		return fmt.Errorf("error trying to determine parent folder: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()

	folder, err := parent.CreateFolder(ctx, path.Base(p))
	if err != nil {
		return fmt.Errorf("error creating folder: %s", err)
	}

	d.SetId(folder.Reference().Value)

	// Apply any pending tags now
	if tagsClient != nil {
		if err := processTagDiff(tagsClient, d, folder); err != nil {
			return fmt.Errorf("error updating tags: %s", err)
		}
	}

	return resourceVSphereFolderRead(d, meta)
}

func resourceVSphereFolderRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	folder, err := folderFromID(client, d.Id())
	if err != nil {
		return fmt.Errorf("cannot locate folder: %s", err)
	}

	// Determine the folder type first. We use the folder as the source of truth
	// here versus the state so that we can support import.
	ft, err := findFolderType(folder)
	if err != nil {
		return fmt.Errorf("cannot determine folder type: %s", err)
	}

	// Again, to support a clean import (which is done off of absolute path to
	// the folder), we discover the datacenter from the path (if it's a thing).
	var dc *object.Datacenter
	p := folder.InventoryPath
	if ft != vSphereFolderTypeDatacenter {
		particle := rootPathParticle(ft)
		dcp, err := particle.SplitDatacenter(p)
		if err != nil {
			return fmt.Errorf("cannot determine datacenter path: %s", err)
		}
		dc, err = getDatacenter(client, dcp)
		if err != nil {
			return fmt.Errorf("cannot find datacenter from path %q: %s", dcp, err)
		}
		relative, err := particle.SplitRelative(p)
		if err != nil {
			return fmt.Errorf("cannot determine relative folder path: %s", err)
		}
		p = relative
	}

	d.Set("path", normalizeFolderPath(p))
	d.Set("type", ft)
	if dc != nil {
		d.Set("datacenter_id", dc.Reference().Value)
	}

	// Read tags if we have the ability to do so
	if tagsClient, _ := meta.(*VSphereClient).TagsClient(); tagsClient != nil {
		if err := readTagsForResource(tagsClient, folder, d); err != nil {
			return fmt.Errorf("error reading tags: %s", err)
		}
	}

	return nil
}

func resourceVSphereFolderUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	tagsClient, err := tagsClientIfDefined(d, meta)
	if err != nil {
		return err
	}

	folder, err := folderFromID(client, d.Id())
	if err != nil {
		return fmt.Errorf("cannot locate folder: %s", err)
	}

	// Apply any pending tags first as it's the lesser expensive of the two
	// operations
	if tagsClient != nil {
		if err := processTagDiff(tagsClient, d, folder); err != nil {
			return fmt.Errorf("error updating tags: %s", err)
		}
	}

	var dc *object.Datacenter
	if dcID, ok := d.GetOk("datacenter_id"); ok {
		var err error
		dc, err = datacenterFromID(client, dcID.(string))
		if err != nil {
			return fmt.Errorf("cannot locate datacenter: %s", err)
		}
	}

	if d.HasChange("path") {
		// The path has changed, which could mean either a change in parent, a
		// change in name, or both.
		ft := vSphereFolderType(d.Get("type").(string))
		oldp, newp := d.GetChange("path")
		oldpa, err := parentFolderFromPath(client, oldp.(string), ft, dc)
		if err != nil {
			return fmt.Errorf("error parsing parent folder from path %q: %s", oldp.(string), err)
		}
		newpa, err := parentFolderFromPath(client, newp.(string), ft, dc)
		if err != nil {
			return fmt.Errorf("error parsing parent folder from path %q: %s", newp.(string), err)
		}
		oldn := path.Base(oldp.(string))
		newn := path.Base(newp.(string))

		if oldn != newn {
			// Folder base name has changed and needs a rename
			if err := renameObject(client, folder.Reference(), newn); err != nil {
				return fmt.Errorf("could not rename folder: %s", err)
			}
		}
		if oldpa.Reference().Value != newpa.Reference().Value {
			// The parent folder has changed - we need to move the folder into the
			// new path
			ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
			defer cancel()
			task, err := newpa.MoveInto(ctx, []types.ManagedObjectReference{folder.Reference()})
			if err != nil {
				return fmt.Errorf("could not move folder: %s", err)
			}
			tctx, tcancel := context.WithTimeout(context.Background(), defaultAPITimeout)
			defer tcancel()
			if err := task.Wait(tctx); err != nil {
				return fmt.Errorf("error on waiting for move task completion: %s", err)
			}
		}
	}

	return resourceVSphereFolderRead(d, meta)
}

func resourceVSphereFolderDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*VSphereClient).vimClient
	folder, err := folderFromID(client, d.Id())
	if err != nil {
		return fmt.Errorf("cannot locate folder: %s", err)
	}

	// We don't destroy if the folder has children. This might be flaggable in
	// the future, but I don't think it's necessary at this point in time -
	// better to have hardcoded safe behavior than hardcoded unsafe behavior.
	ne, err := folderHasChildren(folder)
	if err != nil {
		return fmt.Errorf("error checking for folder contents: %s", err)
	}
	if ne {
		return errors.New("folder is not empty, please remove all items before deleting")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	task, err := folder.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("cannot delete folder: %s", err)
	}
	tctx, tcancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer tcancel()
	if err := task.Wait(tctx); err != nil {
		return fmt.Errorf("error on waiting for deletion task completion: %s", err)
	}

	return nil
}

func resourceVSphereFolderImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// Our subject is the full path to a specific folder, for which we just get
	// the MOID for and then pass off to Read. Easy peasy.
	p := d.Id()
	if !strings.HasPrefix(p, "/") {
		return nil, errors.New("path must start with a trailing slash")
	}
	client := meta.(*VSphereClient).vimClient
	p = normalizeFolderPath(p)
	folder, err := folderFromAbsolutePath(client, p)
	if err != nil {
		return nil, err
	}
	d.SetId(folder.Reference().Value)
	return []*schema.ResourceData{d}, nil
}
