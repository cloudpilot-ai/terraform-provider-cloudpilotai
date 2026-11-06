package eks

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"reflect"
	"strings"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"sigs.k8s.io/yaml"
)

type userDataPart struct {
	header textproto.MIMEHeader
	body   string
}

func preserveMemoryExtensionUserData(configured, remote types.String, enabled *bool) types.String {
	if enabled == nil || configured.IsNull() || configured.IsUnknown() || remote.IsNull() || remote.IsUnknown() {
		return remote
	}
	remoteParts, managed, ok := userDataParts(remote.ValueString())
	if !ok || *enabled != managed {
		return remote
	}
	configuredParts, _, ok := userDataParts(configured.ValueString())
	if ok && reflect.DeepEqual(configuredParts, remoteParts) {
		return configured
	}
	return remote
}

func memoryExtensionEnabled(ctx context.Context, policy customfield.NestedObject[api.MemoryExtensionModel]) *bool {
	if policy.IsNull() || policy.IsUnknown() {
		return nil
	}
	value, diags := policy.Value(ctx)
	if diags.HasError() || value == nil || value.Enabled.IsNull() || value.Enabled.IsUnknown() {
		return nil
	}
	enabled := value.Enabled.ValueBool()
	return &enabled
}

func userDataParts(value string) ([]userDataPart, bool, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false, true
	}
	if !strings.HasPrefix(trimmed, "MIME-Version:") && !strings.HasPrefix(trimmed, "Content-Type:") {
		header := textproto.MIMEHeader{"Content-Type": {plainUserDataContentType(value)}}
		return []userDataPart{{header: header, body: normalizeUserDataBody(value)}}, false, true
	}
	message, err := mail.ReadMessage(strings.NewReader(trimmed))
	if err != nil {
		return nil, false, false
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
		return nil, false, false
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	var parts []userDataPart
	managed := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return parts, managed, true
		}
		if err != nil {
			return nil, false, false
		}
		body, err := io.ReadAll(part)
		if err != nil {
			return nil, false, false
		}
		if part.Header.Get("X-CloudPilot-Managed-Feature") == "memory-extension" {
			managed = true
			continue
		}
		contentType, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			return nil, false, false
		}
		part.Header.Set("Content-Type", mime.FormatMediaType(contentType, params))
		parts = append(parts, userDataPart{header: part.Header, body: normalizeUserDataBody(string(body))})
	}
}

func normalizeUserDataBody(value string) string {
	return strings.ReplaceAll(value, "\r", "")
}

func plainUserDataContentType(value string) string {
	firstLine := strings.TrimSpace(strings.SplitN(strings.TrimSpace(value), "\n", 2)[0])
	contentTypes := map[string]string{
		"#cloud-config":         "text/cloud-config; charset=us-ascii",
		"#cloud-config-jsonp":   "text/cloud-config-jsonp",
		"#cloud-boothook":       "text/cloud-boothook",
		"#cloud-config-archive": "text/cloud-config-archive",
		"## template: jinja":    "text/jinja2",
		"#include":              "text/x-include-url",
		"#include-once":         "text/x-include-once-url",
		"#part-handler":         "text/part-handler",
		"#upstart-job":          "text/upstart-job",
	}
	if contentType, ok := contentTypes[firstLine]; ok {
		return contentType
	}
	if !strings.HasPrefix(firstLine, "#!") {
		var document map[string]any
		if yaml.Unmarshal([]byte(strings.TrimSpace(value)), &document) == nil {
			return "application/node.eks.aws"
		}
	}
	return "text/x-shellscript; charset=us-ascii"
}
