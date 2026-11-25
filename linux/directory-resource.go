package linux

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/spf13/cast"
)

const (
	attrDirectoryProviderOverride = "provider_override"
	attrDirectoryPath             = "path"
	attrDirectoryOwner            = "owner"
	attrDirectoryGroup            = "group"
	attrDirectoryMode             = "mode"
	attrDirectoryOverwrite        = "overwrite"
	attrDirectoryRecyclePath      = "recycle_path"
)

var schemaDirectoryResource = map[string]*schema.Schema{
	attrDirectoryProviderOverride: {
		Description: descProviderOverride,
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: subSchemaProviderOverride,
		},
	},

	attrDirectoryPath: {
		Description: "Absolute path of the directory. The parent directory is created as needed. Changing it moves the " +
			"contents of the current directory to the new one, except hidden entries (names starting with a dot), " +
			"which are lost.",
		Type:     schema.TypeString,
		Required: true,
	},

	attrDirectoryOwner: {
		Description: "ID of the user that owns the directory. Defaults to `0`.",
		Type:        schema.TypeInt,
		Optional:    true,
		Default:     0,
	},
	attrDirectoryGroup: {
		Description: "ID of the group that owns the directory. Defaults to `0`.",
		Type:        schema.TypeInt,
		Optional:    true,
		Default:     0,
	},
	attrDirectoryMode: {
		Description:  "Directory mode as octal digits, for example `755`. Defaults to `755`.",
		Type:         schema.TypeString,
		Optional:     true,
		Default:      "755",
		ValidateFunc: validation.StringMatch(regexp.MustCompile("[0-7]{3}"), "Invalid linux permission"),
	},
	attrDirectoryOverwrite: {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     false,
		Description: "If `true`, an existing directory on the remote is reused on create or update, and its owner and mode are set, instead of being an error. Its contents are never touched. Defaults to `false`.",
	},
	attrDirectoryRecyclePath: {
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "",
		Description: "Absolute path of a directory in which a directory named after the current unix timestamp is created to hold the directory when it is destroyed. When empty, the directory is deleted. Defaults to an empty string.",
	},
}

type handlerDirectoryResource struct{}

func (handlerDirectoryResource) newDirectory(rd *schema.ResourceData) (d *directory) {
	if rd == nil {
		return
	}
	d = &directory{
		path: cast.ToString(rd.Get(attrDirectoryPath)),
		permission: permission{
			owner: cast.ToUint16(rd.Get(attrDirectoryOwner)),
			group: cast.ToUint16(rd.Get(attrDirectoryGroup)),
			mode:  cast.ToString(rd.Get(attrDirectoryMode)),
		},
		overwrite:   cast.ToBool(rd.Get(attrDirectoryOverwrite)),
		recyclePath: cast.ToString(rd.Get(attrDirectoryRecyclePath)),
	}
	return
}

func (handlerDirectoryResource) newDiffedDirectory(rd *schema.ResourceData) (old, new *directory) {
	if rd == nil {
		return
	}
	old, new = &directory{}, &directory{}

	o, n := rd.GetChange(attrDirectoryPath)
	old.path, new.path = cast.ToString(o), cast.ToString(n)

	o, n = rd.GetChange(attrDirectoryOwner)
	old.permission.owner, new.permission.owner = cast.ToUint16(o), cast.ToUint16(n)

	o, n = rd.GetChange(attrDirectoryGroup)
	old.permission.group, new.permission.group = cast.ToUint16(o), cast.ToUint16(n)

	o, n = rd.GetChange(attrDirectoryMode)
	old.permission.mode, new.permission.mode = cast.ToString(o), cast.ToString(n)

	o, n = rd.GetChange(attrDirectoryOverwrite)
	old.overwrite, new.overwrite = cast.ToBool(o), cast.ToBool(n)

	o, n = rd.GetChange(attrDirectoryRecyclePath)
	old.recyclePath, new.recyclePath = cast.ToString(o), cast.ToString(n)
	return
}

func (handlerDirectoryResource) updateResourceData(d *directory, rd *schema.ResourceData) (err error) {
	if d == nil {
		rd.SetId("")
		return
	}

	if err = rd.Set(attrDirectoryPath, d.path); err != nil {
		return
	}
	if err = rd.Set(attrDirectoryOwner, d.permission.owner); err != nil {
		return
	}
	if err = rd.Set(attrDirectoryGroup, d.permission.group); err != nil {
		return
	}
	if err = rd.Set(attrDirectoryMode, d.permission.mode); err != nil {
		return
	}
	if err = rd.Set(attrDirectoryOverwrite, d.overwrite); err != nil {
		return
	}
	if err = rd.Set(attrDirectoryRecyclePath, d.recyclePath); err != nil {
		return
	}
	return
}

func (h handlerDirectoryResource) Read(ctx context.Context, rd *schema.ResourceData, meta interface{}) (dg diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	d, err := l.readDirectory(ctx, cast.ToString(rd.Get(attrDirectoryPath)))
	if errors.Is(err, errPathNotExist) {
		rd.SetId("")
		return
	}
	if err != nil {
		return diag.FromErr(err)
	}

	d.overwrite = cast.ToBool(rd.Get(attrDirectoryOverwrite))
	d.recyclePath = cast.ToString(rd.Get(attrDirectoryRecyclePath))
	if err = h.updateResourceData(d, rd); err != nil {
		return diag.FromErr(err)
	}
	return
}

func (h handlerDirectoryResource) Create(ctx context.Context, rd *schema.ResourceData, meta interface{}) (dg diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	d := h.newDirectory(rd)
	if err := l.createDirectory(ctx, d); err != nil {
		return diag.FromErr(err)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return diag.FromErr(err)
	}

	rd.SetId(id.String())
	return h.Read(ctx, rd, meta)
}

func (h handlerDirectoryResource) Update(ctx context.Context, rd *schema.ResourceData, meta interface{}) (dg diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	old, new := h.newDiffedDirectory(rd)
	err = l.updateDirectory(ctx, old, new)
	if err != nil {
		_ = h.updateResourceData(old, rd) // WARN: see https://github.com/hashicorp/terraform-plugin-sdk/issues/476
		return diag.FromErr(err)
	}

	return h.Read(ctx, rd, meta)
}

func (h handlerDirectoryResource) Delete(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := l.deleteDirectory(ctx, h.newDirectory(rd)); err != nil {
		return diag.FromErr(err)
	}
	return
}

func directoryResource() *schema.Resource {
	var hdr handlerDirectoryResource
	return &schema.Resource{
		Description:   "Manages a directory on a Linux host over SSH, with support for the Terraform update lifecycle.",
		Schema:        schemaDirectoryResource,
		CreateContext: hdr.Create,
		ReadContext:   hdr.Read,
		UpdateContext: hdr.Update,
		DeleteContext: hdr.Delete,
	}
}
