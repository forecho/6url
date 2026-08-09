package notifications

import (
	"strings"
	"testing"
)

func TestRenderEscapesApplicantContent(t *testing.T) {
	_, textBody, htmlBody := render("zh-CN", "submission.received", Payload{ApplicantName: "<script>", Name: "A&B", Status: "received", StatusURL: "https://6url.net/status#token=x"})
	if strings.Contains(htmlBody, "<script>") || !strings.Contains(htmlBody, "&lt;script&gt;") || !strings.Contains(htmlBody, "A&amp;B") {
		t.Fatalf("HTML was not escaped: %s", htmlBody)
	}
	if !strings.Contains(textBody, "<script>") {
		t.Fatal("plain-text fallback lost content")
	}
}
