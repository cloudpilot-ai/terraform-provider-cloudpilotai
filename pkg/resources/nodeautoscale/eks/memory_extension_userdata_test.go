package eks

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/cloudpilot-ai/api"
	customfield "github.com/cloudpilot-ai/terraform-provider-cloudpilotai/third_party/cloudflare/customfield"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMemoryExtensionUserDataPreservesEquivalentUserContent(t *testing.T) {
	script := "#!/bin/bash\necho example\n"
	enabled := true
	for _, configured := range []string{script, strings.ReplaceAll(script, "\n", "\r\n"), "#!/bin/bash\necho one\recho two\n", "#cloud-config\npackages: [curl]\n", "apiVersion: node.eks.aws/v1alpha1\nkind: NodeConfig\n", ""} {
		parts, _, ok := userDataParts(configured)
		if !ok {
			t.Fatal("invalid test input")
		}
		remote := memoryExtensionMIME(t, "server", parts, true)
		got := preserveMemoryExtensionUserData(types.StringValue(configured), types.StringValue(remote), &enabled)
		if got.ValueString() != configured {
			t.Fatalf("configured user data was replaced with its wrapper: %q", configured)
		}
	}
	parts := []userDataPart{
		{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript; charset=us-ascii"}, "X-Example": {"original"}}, body: script},
		{header: textproto.MIMEHeader{"Content-Type": {"application/node.eks.aws"}}, body: "kind: NodeConfig\n"},
	}
	configured := memoryExtensionMIME(t, "configured", parts, false)
	remote := memoryExtensionMIME(t, "server", parts, true)
	if got := preserveMemoryExtensionUserData(types.StringValue(configured), types.StringValue(remote), &enabled); got.ValueString() != configured {
		t.Fatal("equivalent MIME user data was not preserved")
	}
}

func TestMemoryExtensionUserDataKeepsGenuineDrift(t *testing.T) {
	configured := "#!/bin/bash\necho example\n"
	enabled := true
	for _, tc := range []struct {
		name    string
		parts   []userDataPart
		managed bool
	}{
		{name: "changed body", parts: []userDataPart{{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript; charset=us-ascii"}}, body: "#!/bin/bash\necho changed\n"}}, managed: true},
		{name: "changed type", parts: []userDataPart{{header: textproto.MIMEHeader{"Content-Type": {"text/cloud-config; charset=us-ascii"}}, body: configured}}, managed: true},
		{name: "changed header", parts: []userDataPart{{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript; charset=us-ascii"}, "X-Example": {"new"}}, body: configured}}, managed: true},
		{name: "extra part", parts: []userDataPart{{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript; charset=us-ascii"}}, body: configured}, {header: textproto.MIMEHeader{"Content-Type": {"text/plain"}}, body: "extra"}}, managed: true},
		{name: "missing ownership marker", parts: []userDataPart{{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript; charset=us-ascii"}}, body: configured}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := memoryExtensionMIME(t, "server", tc.parts, tc.managed)
			if got := preserveMemoryExtensionUserData(types.StringValue(configured), types.StringValue(remote), &enabled); got.ValueString() != remote {
				t.Fatal("genuine user data drift was suppressed")
			}
		})
	}
	remote := "MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=missing\n\ntruncated"
	if got := preserveMemoryExtensionUserData(types.StringValue(configured), types.StringValue(remote), &enabled); got.ValueString() != remote {
		t.Fatal("malformed MIME was suppressed")
	}
}

func TestMemoryExtensionUserDataStatePreservesManagement(t *testing.T) {
	ctx := context.Background()
	configured := "#!/bin/bash\necho example\n"
	parts, _, _ := userDataParts(configured)
	wrapped := memoryExtensionMIME(t, "server", parts, true)
	unwrapped := memoryExtensionMIME(t, "server", parts, false)
	for _, tc := range []struct {
		name       string
		configured types.String
		enabled    bool
		remote     string
		want       types.String
	}{
		{name: "enabled", configured: types.StringValue(configured), enabled: true, remote: wrapped, want: types.StringValue(configured)},
		{name: "disabled", configured: types.StringValue(configured), remote: unwrapped, want: types.StringValue(configured)},
		{name: "disabled bootstrap drift", configured: types.StringValue(configured), remote: wrapped, want: types.StringValue(wrapped)},
		{name: "unmanaged", configured: types.StringNull(), enabled: true, remote: wrapped, want: types.StringNull()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := api.EC2NodeClassModel{UserData: types.StringValue(tc.remote), MemoryExtension: customfield.NewObjectMust(ctx, &api.MemoryExtensionModel{Enabled: types.BoolValue(tc.enabled)})}
			got, err := preserveNodeClassStateRepresentation(ctx, remote, api.EC2NodeClassModel{UserData: tc.configured})
			if err != nil || !got.UserData.Equal(tc.want) {
				t.Fatalf("user data = %v, want %v, error = %v", got.UserData, tc.want, err)
			}
		})
	}
}

func memoryExtensionMIME(t *testing.T, boundary string, parts []userDataPart, managed bool) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.SetBoundary(boundary); err != nil {
		t.Fatal(err)
	}
	if managed {
		parts = append(append([]userDataPart(nil), parts...), userDataPart{header: textproto.MIMEHeader{"Content-Type": {"text/x-shellscript"}, "X-Cloudpilot-Managed-Feature": {"memory-extension"}}, body: "managed bootstrap"})
	}
	for _, part := range parts {
		out, err := writer.CreatePart(part.header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write([]byte(part.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=%s\n\n%s", boundary, strings.ReplaceAll(body.String(), "\r", ""))
}
