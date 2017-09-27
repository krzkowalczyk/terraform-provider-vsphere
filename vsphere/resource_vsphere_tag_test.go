package vsphere

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccResourceVSphereTag(t *testing.T) {
	var tp *testing.T
	testAccResourceVSphereTagCases := []struct {
		name     string
		testCase resource.TestCase
	}{
		{
			"basic",
			resource.TestCase{
				PreCheck: func() {
					testAccPreCheck(tp)
				},
				Providers:    testAccProviders,
				CheckDestroy: testAccResourceVSphereTagExists(false),
				Steps: []resource.TestStep{
					{
						Config: testAccResourceVSphereTagConfigBasic,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
							testAccResourceVSphereTagHasName("terraform-test-tag"),
							testAccResourceVSphereTagHasDescription("Managed by Terraform"),
							testAccResourceVSphereTagHasCategory(),
						),
					},
				},
			},
		},
		{
			"change name",
			resource.TestCase{
				PreCheck: func() {
					testAccPreCheck(tp)
				},
				Providers:    testAccProviders,
				CheckDestroy: testAccResourceVSphereTagExists(false),
				Steps: []resource.TestStep{
					{
						Config: testAccResourceVSphereTagConfigBasic,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
						),
					},
					{
						Config: testAccResourceVSphereTagConfigAltName,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
							testAccResourceVSphereTagHasName("terraform-test-tag-renamed"),
						),
					},
				},
			},
		},
		{
			"change description",
			resource.TestCase{
				PreCheck: func() {
					testAccPreCheck(tp)
				},
				Providers:    testAccProviders,
				CheckDestroy: testAccResourceVSphereTagExists(false),
				Steps: []resource.TestStep{
					{
						Config: testAccResourceVSphereTagConfigBasic,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
						),
					},
					{
						Config: testAccResourceVSphereTagConfigAltDescription,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
							testAccResourceVSphereTagHasDescription("Still managed by Terraform"),
						),
					},
				},
			},
		},
		{
			"import",
			resource.TestCase{
				PreCheck: func() {
					testAccPreCheck(tp)
				},
				Providers:    testAccProviders,
				CheckDestroy: testAccResourceVSphereTagExists(false),
				Steps: []resource.TestStep{
					{
						Config: testAccResourceVSphereTagConfigBasic,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
						),
					},
					{
						ResourceName:      "vsphere_tag.terraform-test-tag",
						ImportState:       true,
						ImportStateVerify: true,
						ImportStateIdFunc: func(s *terraform.State) (string, error) {
							cat, err := testGetTagCategory(s, "terraform-test-category")
							if err != nil {
								return "", err
							}
							tag, err := testGetTag(s, "terraform-test-tag")
							if err != nil {
								return "", err
							}
							m := make(map[string]string)
							m["category_name"] = cat.Name
							m["tag_name"] = tag.Name
							b, err := json.Marshal(m)
							if err != nil {
								return "", err
							}

							return string(b), nil
						},
						Config: testAccResourceVSphereTagConfigBasic,
						Check: resource.ComposeTestCheckFunc(
							testAccResourceVSphereTagExists(true),
						),
					},
				},
			},
		},
	}

	for _, tc := range testAccResourceVSphereTagCases {
		t.Run(tc.name, func(t *testing.T) {
			tp = t
			resource.Test(t, tc.testCase)
		})
	}
}

func testAccResourceVSphereTagExists(expected bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		_, err := testGetTag(s, "terraform-test-tag")
		if err != nil {
			if strings.Contains(err.Error(), "Status code: 404") && !expected {
				// Expected missing
				return nil
			}
			return err
		}
		if !expected {
			return errors.New("expected tag to be missing")
		}
		return nil
	}
}

func testAccResourceVSphereTagHasName(expected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		tag, err := testGetTag(s, "terraform-test-tag")
		if err != nil {
			return err
		}
		actual := tag.Name
		if expected != actual {
			return fmt.Errorf("expected name to be %q, got %q", expected, actual)
		}
		return nil
	}
}

func testAccResourceVSphereTagHasDescription(expected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		tag, err := testGetTag(s, "terraform-test-tag")
		if err != nil {
			return err
		}
		actual := tag.Description
		if expected != actual {
			return fmt.Errorf("expected description to be %q, got %q", expected, actual)
		}
		return nil
	}
}

func testAccResourceVSphereTagHasCategory() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		tag, err := testGetTag(s, "terraform-test-tag")
		if err != nil {
			return err
		}
		category, err := testGetTagCategory(s, "terraform-test-category")
		if err != nil {
			return err
		}

		expected := category.ID
		actual := tag.CategoryID
		if expected != actual {
			return fmt.Errorf("expected ID to be %q, got %q", expected, actual)
		}
		return nil
	}
}

const testAccResourceVSphereTagConfigBasic = `
resource "vsphere_tag_category" "terraform-test-category" {
  name        = "terraform-test-category"
  cardinality = "SINGLE"

  associable_types = [
    "All",
  ]
}

resource "vsphere_tag" "terraform-test-tag" {
  name        = "terraform-test-tag"
  description = "Managed by Terraform"
  category_id = "${vsphere_tag_category.terraform-test-category.id}"
}
`

const testAccResourceVSphereTagConfigAltName = `
resource "vsphere_tag_category" "terraform-test-category" {
  name        = "terraform-test-category"
  cardinality = "SINGLE"

  associable_types = [
    "All",
  ]
}

resource "vsphere_tag" "terraform-test-tag" {
  name        = "terraform-test-tag-renamed"
  description = "Managed by Terraform"
  category_id = "${vsphere_tag_category.terraform-test-category.id}"
}
`

const testAccResourceVSphereTagConfigAltDescription = `
resource "vsphere_tag_category" "terraform-test-category" {
  name        = "terraform-test-category"
  cardinality = "SINGLE"

  associable_types = [
    "All",
  ]
}

resource "vsphere_tag" "terraform-test-tag" {
  name        = "terraform-test-tag"
  description = "Still managed by Terraform"
  category_id = "${vsphere_tag_category.terraform-test-category.id}"
}
`
