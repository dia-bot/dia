package storage

import (
	"encoding/hex"
	"testing"
)

// Validates the SigV4 signing-key derivation. The expected value was computed
// independently from the canonical AWS algorithm (the documented Python
// getSignatureKey) for these inputs, pinning the crypto chain the live signing
// depends on.
func TestSigningKeyVector(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte("20150830"))
	kRegion := hmacSHA256(kDate, []byte("us-east-1"))
	kService := hmacSHA256(kRegion, []byte("iam"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	got := hex.EncodeToString(kSigning)
	want := "2c94c0cf5378ada6887f09bb697df8fc0affdb34ba1cdd5bda32b664bd55b73c"
	if got != want {
		t.Fatalf("signing key mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestEncodeKey(t *testing.T) {
	cases := map[string]string{
		"a/b/c.png":             "a/b/c.png",
		"uploads/g1/wei rd.png": "uploads/g1/wei%20rd.png",
		"/leading/slash.png":    "leading/slash.png",
		"a+b&c.png":             "a%2Bb%26c.png",
	}
	for in, want := range cases {
		if got := encodeKey(in); got != want {
			t.Errorf("encodeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublicURL(t *testing.T) {
	// path-style, no public base → endpoint/bucket/key
	s, err := New(Config{
		Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "cards",
		AccessKey: "k", SecretKey: "s", ForcePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.PublicURL("uploads/x.png"), "https://s3.example.com/cards/uploads/x.png"; got != want {
		t.Errorf("path-style PublicURL = %q, want %q", got, want)
	}

	// virtual-host, custom public base → base/key
	s2, _ := New(Config{
		Endpoint: "https://s3.example.com", Region: "auto", Bucket: "cards",
		AccessKey: "k", SecretKey: "s", PublicBaseURL: "https://cdn.example.com/",
	})
	if got, want := s2.PublicURL("uploads/x.png"), "https://cdn.example.com/uploads/x.png"; got != want {
		t.Errorf("cdn PublicURL = %q, want %q", got, want)
	}
}
