package vsphere

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// vSphereFolderType is an enumeration type for vSphere folder types.
type vSphereFolderType string

// The following are constants for the 5 vSphere folder types - these are used
// to help determine base paths and also to validate folder types in the
// vsphere_folder resource.
const (
	vSphereFolderTypeVM        = vSphereFolderType("vm")
	vSphereFolderTypeNetwork   = vSphereFolderType("network")
	vSphereFolderTypeHost      = vSphereFolderType("host")
	vSphereFolderTypeDatastore = vSphereFolderType("datastore")

	// vSphereFolderTypeDatacenter is a special folder type - it does not get a
	// root path particle generated for it as it is an integral part of the path
	// generation process, but is defined so that it can be properly referenced
	// and used in validation.
	vSphereFolderTypeDatacenter = vSphereFolderType("datacenter")
)

// rootPathParticle is the section of a vSphere inventory path that denotes a
// specific kind of inventory item.
type rootPathParticle vSphereFolderType

// String implements Stringer for rootPathParticle.
func (p rootPathParticle) String() string {
	return string(p)
}

// Delimiter returns the path delimiter for the particle, which is basically
// just a particle with a leading slash.
func (p rootPathParticle) Delimiter() string {
	return string("/" + p)
}

// RootFromDatacenter returns the root path for the particle from the given
// datacenter's inventory path.
func (p rootPathParticle) RootFromDatacenter(dc *object.Datacenter) string {
	return dc.InventoryPath + "/" + string(p)
}

// PathFromDatacenter returns the combined result of RootFromDatacenter plus a
// relative path for a given particle and datacenter object.
func (p rootPathParticle) PathFromDatacenter(dc *object.Datacenter, relative string) string {
	return p.RootFromDatacenter(dc) + "/" + relative
}

// SplitDatacenter is a convenience method that splits out the datacenter path
// from the supplied path for the particle.
func (p rootPathParticle) SplitDatacenter(inventoryPath string) (string, error) {
	s := strings.SplitN(inventoryPath, p.Delimiter(), 2)
	if len(s) != 2 {
		return inventoryPath, fmt.Errorf("could not split path %q on %q", inventoryPath, p.Delimiter())
	}
	return s[0], nil
}

// SplitRelative is a convenience method that splits out the relative path from
// the supplied path for the particle.
func (p rootPathParticle) SplitRelative(inventoryPath string) (string, error) {
	s := strings.SplitN(inventoryPath, p.Delimiter(), 2)
	if len(s) != 2 {
		return inventoryPath, fmt.Errorf("could not split path %q on %q", inventoryPath, p.Delimiter())
	}
	return s[1], nil
}

// SplitRelativeFolder is a convenience method that returns the parent folder
// for the result of SplitRelative on the supplied path.
//
// This is generally useful to get the folder for a managed entity, versus getting a full relative path. If you want that, use SplitRelative instead.
func (p rootPathParticle) SplitRelativeFolder(inventoryPath string) (string, error) {
	relative, err := p.SplitRelative(inventoryPath)
	if err != nil {
		return inventoryPath, err
	}
	return path.Dir(relative), nil
}

// NewRootFromPath takes the datacenter path for a specific entity, and then
// appends the new particle supplied.
func (p rootPathParticle) NewRootFromPath(inventoryPath string, newParticle rootPathParticle) (string, error) {
	dcPath, err := p.SplitDatacenter(inventoryPath)
	if err != nil {
		return inventoryPath, err
	}
	return fmt.Sprintf("%s/%s", dcPath, newParticle), nil
}

// PathFromNewRoot takes the datacenter path for a specific entity, and then
// appends the new particle supplied with the new relative path.
//
// As an example, consider a supplied host path "/dc1/host/cluster1/esxi1", and
// a supplied datastore folder relative path of "/foo/bar".  This function will
// split off the datacenter section of the path (/dc1) and combine it with the
// datastore folder with the proper delimiter. The resulting path will be
// "/dc1/datastore/foo/bar".
func (p rootPathParticle) PathFromNewRoot(inventoryPath string, newParticle rootPathParticle, relative string) (string, error) {
	rootPath, err := p.NewRootFromPath(inventoryPath, newParticle)
	if err != nil {
		return inventoryPath, err
	}
	return path.Clean(fmt.Sprintf("%s/%s", rootPath, relative)), nil
}

const (
	rootPathParticleVM        = rootPathParticle(vSphereFolderTypeVM)
	rootPathParticleNetwork   = rootPathParticle(vSphereFolderTypeNetwork)
	rootPathParticleHost      = rootPathParticle(vSphereFolderTypeHost)
	rootPathParticleDatastore = rootPathParticle(vSphereFolderTypeDatastore)
)

// datacenterPathFromHostSystemID returns the datacenter section of a
// HostSystem's inventory path.
func datacenterPathFromHostSystemID(client *govmomi.Client, hsID string) (string, error) {
	hs, err := hostSystemFromID(client, hsID)
	if err != nil {
		return "", err
	}
	return rootPathParticleHost.SplitDatacenter(hs.InventoryPath)
}

// datastoreRootPathFromHostSystemID returns the root datastore folder path
// for a specific host system ID.
func datastoreRootPathFromHostSystemID(client *govmomi.Client, hsID string) (string, error) {
	hs, err := hostSystemFromID(client, hsID)
	if err != nil {
		return "", err
	}
	return rootPathParticleHost.NewRootFromPath(hs.InventoryPath, rootPathParticleDatastore)
}

// folderFromAbsolutePath returns an *object.Folder from a given absolute path.
// If no such folder is found, an appropriate error will be returned.
func folderFromAbsolutePath(client *govmomi.Client, path string) (*object.Folder, error) {
	finder := find.NewFinder(client.Client, false)
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	folder, err := finder.Folder(ctx, path)
	if err != nil {
		return nil, err
	}
	return folder, nil
}

// folderFromObject returns an *object.Folder from a given object of specific
// types, and relative path of a type defined in folderType. If no such folder
// is found, an appropriate error will be returned.
//
// The list of supported object types will grow as the provider supports more
// resources.
func folderFromObject(client *govmomi.Client, obj interface{}, folderType rootPathParticle, relative string) (*object.Folder, error) {
	if err := validateVirtualCenter(client); err != nil {
		return nil, err
	}
	var p string
	var err error
	switch o := obj.(type) {
	case (*object.Datastore):
		p, err = rootPathParticleDatastore.PathFromNewRoot(o.InventoryPath, folderType, relative)
	case (*object.HostSystem):
		p, err = rootPathParticleHost.PathFromNewRoot(o.InventoryPath, folderType, relative)
	default:
		return nil, fmt.Errorf("unsupported object type %T", o)
	}
	if err != nil {
		return nil, err
	}
	return folderFromAbsolutePath(client, p)
}

// datastoreFolderFromObject returns an *object.Folder from a given object,
// and relative datastore folder path. If no such folder is found, of if it is
// not a datastore folder, an appropriate error will be returned.
func datastoreFolderFromObject(client *govmomi.Client, obj interface{}, relative string) (*object.Folder, error) {
	folder, err := folderFromObject(client, obj, rootPathParticleDatastore, relative)
	if err != nil {
		return nil, err
	}

	return validateDatastoreFolder(folder)
}

// validateDatastoreFolder checks to make sure the folder is a datastore
// folder, and returns it if it is not, or an error if it isn't.
func validateDatastoreFolder(folder *object.Folder) (*object.Folder, error) {
	ft, err := findFolderType(folder)
	if err != nil {
		return nil, err
	}
	if ft != vSphereFolderTypeDatastore {
		return nil, fmt.Errorf("%q is not a datastore folder", folder.InventoryPath)
	}
	return folder, nil
}

// pathIsEmpty checks a folder path to see if it's "empty" (ie: would resolve
// to the root inventory path for a given type in a datacenter - "" or "/").
func pathIsEmpty(path string) bool {
	return path == "" || path == "/"
}

// normalizeFolderPath is a SchemaStateFunc that normalizes a folder path.
func normalizeFolderPath(v interface{}) string {
	p := v.(string)
	if pathIsEmpty(p) {
		return ""
	}
	return strings.TrimPrefix(path.Clean(p), "/")
}

// moveObjectToFolder moves a object by reference into a folder.
func moveObjectToFolder(ref types.ManagedObjectReference, folder *object.Folder) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	task, err := folder.MoveInto(ctx, []types.ManagedObjectReference{ref})
	if err != nil {
		return err
	}
	tctx, tcancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer tcancel()
	return task.Wait(tctx)
}

// parentFolderFromPath takes a relative object path (usually a folder), an
// object type, and an optional supplied datacenter, and returns the parent
// *object.Folder if it exists.
//
// The datacenter supplied in dc cannot be nil if the folder type supplied by
// ft is something else other than vSphereFolderTypeDatacenter.
func parentFolderFromPath(c *govmomi.Client, p string, ft vSphereFolderType, dc *object.Datacenter) (*object.Folder, error) {
	var fp string
	if ft == vSphereFolderTypeDatacenter {
		fp = "/" + p
	} else {
		pt := rootPathParticle(ft)
		fp = pt.PathFromDatacenter(dc, p)
	}
	return folderFromAbsolutePath(c, path.Dir(fp))
}

// folderFromID locates a Folder by its managed object reference ID.
func folderFromID(client *govmomi.Client, id string) (*object.Folder, error) {
	finder := find.NewFinder(client.Client, false)

	ref := types.ManagedObjectReference{
		Type:  "Folder",
		Value: id,
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	folder, err := finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return folder.(*object.Folder), nil
}

// folderProperties is a convenience method that wraps fetching the
// Folder MO from its higher-level object.
func folderProperties(folder *object.Folder) (*mo.Folder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	var props mo.Folder
	if err := folder.Properties(ctx, folder.Reference(), nil, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

// findFolderType returns a proper vSphereFolderType for a folder object by checking its child type.
func findFolderType(folder *object.Folder) (vSphereFolderType, error) {
	var ft vSphereFolderType

	props, err := folderProperties(folder)
	if err != nil {
		return ft, err
	}

	ct := props.ChildType
	if ct[0] != "Folder" {
		return ft, fmt.Errorf("expected first childtype node to be Folder, got %s", ct[0])
	}

	switch ct[1] {
	case "Datacenter":
		ft = vSphereFolderTypeDatacenter
	case "ComputeResource":
		ft = vSphereFolderTypeHost
	case "VirtualMachine":
		ft = vSphereFolderTypeVM
	case "Datastore":
		ft = vSphereFolderTypeDatastore
	case "Network":
		ft = vSphereFolderTypeNetwork
	default:
		return ft, fmt.Errorf("unknown folder type: %#v", ct)
	}

	return ft, nil
}

// folderHasChildren checks to see if a folder has any child items and returns
// true if that is the case. This is useful when checking to see if a folder is
// safe to delete - destroying a folder in vSphere destroys *all* children if
// at all possible (including removing virtual machines), so extra verification
// is necessary to prevent accidental removal.
func folderHasChildren(f *object.Folder) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPITimeout)
	defer cancel()
	children, err := f.Children(ctx)
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}
