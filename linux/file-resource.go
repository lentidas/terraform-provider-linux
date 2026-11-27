package linux

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/spf13/cast"
)

const (
	attrFileProviderOverride = "provider_override"
	attrFilePath             = "path"
	attrFileContent          = "content"
	attrFileOwner            = "owner"
	attrFileGroup            = "group"
	attrFileMode             = "mode"
	attrFileIgnoreContent    = "ignore_content"
	attrFileOverwrite        = "overwrite"
	attrFileRecyclePath      = "recycle_path"
)

var schemaFileResource = map[string]*schema.Schema{
	attrFileProviderOverride: {
		Description: descProviderOverride,
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: subSchemaProviderOverride,
		},
	},

	attrFilePath: {
		Description: "Absolute path of the file. The parent directory is created as needed.",
		Type:        schema.TypeString,
		Required:    true,
	},
	attrFileContent: {
		Description: "Content of the file. Defaults to an empty string.",
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "",
		DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
			return cast.ToBool(d.Get(attrFileIgnoreContent))
		},
	},
	attrFileOwner: {
		Description:  "ID of the user that owns the file. Defaults to `0`.",
		Type:         schema.TypeInt,
		Optional:     true,
		Default:      0,
		ValidateFunc: validateID,
	},
	attrFileGroup: {
		Description:  "ID of the group that owns the file. Defaults to `0`.",
		Type:         schema.TypeInt,
		Optional:     true,
		Default:      0,
		ValidateFunc: validateID,
	},
	attrFileMode: {
		Description:      "File mode as three or four octal digits, for example `644`. Defaults to `644`.",
		Type:             schema.TypeString,
		Optional:         true,
		Default:          "644",
		ValidateFunc:     validateMode,
		DiffSuppressFunc: suppressEquivalentMode,
	},
	attrFileIgnoreContent: {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     false,
		Description: "If `true`, `content` is ignored and left out of the diff, so changes to it do not rewrite the file. Defaults to `false`.",
	},
	attrFileOverwrite: {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     false,
		Description: "If `true`, an existing file on the remote is replaced on create or update. Otherwise it is an error. Defaults to `false`.",
	},
	attrFileRecyclePath: {
		Type:        schema.TypeString,
		Optional:    true,
		Default:     "",
		Description: "Absolute path of a directory in which a directory named after the current unix timestamp and a random suffix is created to hold the file when it is destroyed. When empty, the file is deleted. Defaults to an empty string.",
	},
}

type handlerFileResource struct{}

func (handlerFileResource) newFile(rd *schema.ResourceData) (f *file) {
	if rd == nil {
		return
	}
	f = &file{
		path:    cast.ToString(rd.Get(attrFilePath)),
		content: cast.ToString(rd.Get(attrFileContent)),
		permission: permission{
			owner: cast.ToUint32(rd.Get(attrFileOwner)),
			group: cast.ToUint32(rd.Get(attrFileGroup)),
			mode:  cast.ToString(rd.Get(attrFileMode)),
		},
		ignoreContent: cast.ToBool(rd.Get(attrFileIgnoreContent)),
		overwrite:     cast.ToBool(rd.Get(attrFileOverwrite)),
		recyclePath:   cast.ToString(rd.Get(attrFileRecyclePath)),
	}
	return
}

func (handlerFileResource) newDiffedFile(rd *schema.ResourceData) (old, new *file) {
	if rd == nil {
		return
	}
	old, new = &file{}, &file{}

	o, n := rd.GetChange(attrFilePath)
	old.path, new.path = cast.ToString(o), cast.ToString(n)

	o, n = rd.GetChange(attrFileContent)
	old.content, new.content = cast.ToString(o), cast.ToString(n)

	o, n = rd.GetChange(attrFileOwner)
	old.permission.owner, new.permission.owner = cast.ToUint32(o), cast.ToUint32(n)

	o, n = rd.GetChange(attrFileGroup)
	old.permission.group, new.permission.group = cast.ToUint32(o), cast.ToUint32(n)

	o, n = rd.GetChange(attrFileMode)
	old.permission.mode, new.permission.mode = cast.ToString(o), cast.ToString(n)

	o, n = rd.GetChange(attrFileIgnoreContent)
	old.ignoreContent, new.ignoreContent = cast.ToBool(o), cast.ToBool(n)

	o, n = rd.GetChange(attrFileOverwrite)
	old.overwrite, new.overwrite = cast.ToBool(o), cast.ToBool(n)

	o, n = rd.GetChange(attrFileRecyclePath)
	old.recyclePath, new.recyclePath = cast.ToString(o), cast.ToString(n)
	return
}

func (handlerFileResource) updateResourceData(f *file, rd *schema.ResourceData) (err error) {
	if f == nil {
		rd.SetId("")
		return
	}

	if err = rd.Set(attrFilePath, f.path); err != nil {
		return
	}
	if err = rd.Set(attrFileOwner, f.permission.owner); err != nil {
		return
	}
	if err = rd.Set(attrFileGroup, f.permission.group); err != nil {
		return
	}
	if err = rd.Set(attrFileMode, f.permission.mode); err != nil {
		return
	}
	if err = rd.Set(attrFileOverwrite, f.overwrite); err != nil {
		return
	}
	if err = rd.Set(attrFileRecyclePath, f.recyclePath); err != nil {
		return
	}

	if err = rd.Set(attrFileIgnoreContent, f.ignoreContent); err != nil {
		return
	}
	if f.ignoreContent {
		return
	}
	if err = rd.Set(attrFileContent, f.content); err != nil {
		return
	}
	return
}

func (h handlerFileResource) Read(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	f, err := l.readFile(ctx, cast.ToString(rd.Get(attrFilePath)), cast.ToBool(rd.Get(attrFileIgnoreContent)))
	if errors.Is(err, errPathNotExist) {
		rd.SetId("")
		return
	}
	if err != nil {
		return diag.FromErr(err)
	}

	f.overwrite = cast.ToBool(rd.Get(attrFileOverwrite))
	f.recyclePath = cast.ToString(rd.Get(attrFileRecyclePath))
	if err = h.updateResourceData(f, rd); err != nil {
		return diag.FromErr(err)
	}
	return
}

func (h handlerFileResource) Create(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}
	f := h.newFile(rd)
	if err := l.createFile(ctx, f); err != nil {
		return diag.FromErr(err)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return diag.FromErr(err)
	}

	rd.SetId(id.String())
	return h.Read(ctx, rd, meta)
}

func (h handlerFileResource) Update(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	old, new := h.newDiffedFile(rd)
	err = l.updateFile(ctx, old, new)
	if err != nil {
		_ = h.updateResourceData(old, rd) // WARN: see https://github.com/hashicorp/terraform-plugin-sdk/issues/476
		return diag.FromErr(err)
	}

	return h.Read(ctx, rd, meta)
}

func (h handlerFileResource) Delete(ctx context.Context, rd *schema.ResourceData, meta interface{}) (d diag.Diagnostics) {
	l, err := getLinux(meta.(*linuxPool), rd)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := l.deleteFile(ctx, h.newFile(rd)); err != nil {
		return diag.FromErr(err)
	}
	return
}

func fileResource() *schema.Resource {
	var hfr handlerFileResource
	return &schema.Resource{
		Description:   "Manages a file on a Linux host over SSH, with support for the Terraform update lifecycle.",
		Schema:        schemaFileResource,
		CreateContext: hfr.Create,
		ReadContext:   hfr.Read,
		UpdateContext: hfr.Update,
		DeleteContext: hfr.Delete,
	}
}
