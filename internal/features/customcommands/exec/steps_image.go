package exec

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// hImageRender renders a Card Studio template against the scope's vars and
// stores the resulting PNG bytes (base64-encoded) under spec.Into so a later
// reply step can attach them.
func hImageRender(ctx context.Context, h *Halt) error {
	var spec cc.SpecImageRender
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.TemplateID == 0 {
		return errors.New("image_render: template_id required")
	}
	if spec.Into == "" {
		return errors.New("image_render: into required")
	}
	if h.Run.imgRenders.Add(1) > maxImageRendersPerRun {
		return errors.New("image_render: per-run render budget exceeded")
	}
	gid, _ := event.ParseID(h.Run.GuildID)
	tpl, err := h.Deps.Store.GetImageTemplate(ctx, gid, spec.TemplateID)
	if err != nil {
		return err
	}
	// Build the vars map: ctx tokens + any user-supplied overrides (templated).
	vars := h.Scope.CardVars()
	for k, src := range spec.Vars {
		if v, err := cc.EvalTemplated(ctx, src, h.Scope); err == nil {
			vars[k] = v
			// Also expose the unbraced alias so layout vars can be referenced
			// without the surrounding {}.
			vars["{"+k+"}"] = v
		}
	}
	png, err := h.Deps.Imaging.RenderLayoutBytes(ctx, tpl.Layout, vars, nil)
	if err != nil {
		return err
	}
	h.Scope.SetImageBlob(spec.Into, cc.ImageBlob{
		Bytes:       base64.StdEncoding.EncodeToString(png),
		ContentType: "image/png",
		Filename:    tpl.Name + ".png",
	})
	h.SetOutput(map[string]any{"bytes": len(png), "template": tpl.Name})
	return nil
}

// hImageAttach queues a scope-resident image for the next reply step.
func hImageAttach(ctx context.Context, h *Halt) error {
	var spec cc.SpecImageAttach
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.FromVar == "" {
		return errors.New("image_attach: from_var required")
	}
	h.Scope.QueueAttachment(cc.ScopeAttachment{FromVar: spec.FromVar, Filename: spec.Filename})
	return nil
}

// hImageLoad downloads an image (SSRF-guarded) into scope.
func hImageLoad(ctx context.Context, h *Halt) error {
	var spec cc.SpecImageLoad
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Into == "" {
		return errors.New("image_load: into required")
	}
	src, _ := cc.EvalString(ctx, spec.Source, h.Scope)
	src = strings.TrimSpace(src)
	if src == "" {
		return errors.New("image_load: source required")
	}
	max := spec.MaxBytes
	if max <= 0 {
		max = 8 << 20
	}
	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return err
	}
	resp, err := h.Deps.HTTP.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(max)+1))
	if err != nil {
		return err
	}
	if len(body) > max {
		return errors.New("image_load: source exceeds max_bytes")
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Scope.SetImageBlob(spec.Into, cc.ImageBlob{
		Bytes:       base64.StdEncoding.EncodeToString(body),
		ContentType: ct,
		Filename:    spec.Into + ".bin",
	})
	h.SetOutput(map[string]any{"bytes": len(body), "content_type": ct})
	return nil
}
