package awsmonitoringconfig

import "testing"

func TestAccountIDFromRoleArn(t *testing.T) {
	cases := map[string]string{
		"":                                                         "",
		"arn:aws:iam::123456789012:role/MyRole":                    "123456789012",
		"arn:aws:iam::123456789012:role/path/to/SomeRole":          "123456789012",
		"arn:aws:iam::aws:role/AWSServiceRole":                     "aws",
		"not-an-arn":                                               "",
		"arn:aws:s3:::bucket":                                      "",
	}
	for arn, want := range cases {
		got := AccountIDFromRoleArn(arn)
		if got != want {
			t.Errorf("AccountIDFromRoleArn(%q) = %q, want %q", arn, got, want)
		}
	}
}

func TestParseRequiredRegions(t *testing.T) {
	if _, err := ParseRequiredRegions(""); err == nil {
		t.Errorf("expected error for empty input")
	}
	if _, err := ParseRequiredRegions("  ,  "); err == nil {
		t.Errorf("expected error for whitespace-only input")
	}
	got, err := ParseRequiredRegions("us-east-1, eu-central-1 , ap-south-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"us-east-1", "eu-central-1", "ap-south-1"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestSplitCSV(t *testing.T) {
	got := SplitCSV(", a , ,b,, c ,")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idx %d: %q vs %q", i, got[i], want[i])
		}
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "0.1.6", 1},
		{"0.1.6", "1.0.0", -1},
		{"1.10.0", "1.2.0", 1},
		{"2.0", "1.99.99", 1},
	}
	for _, tc := range cases {
		got := compareVersion(tc.a, tc.b)
		if (got > 0 && tc.want <= 0) || (got < 0 && tc.want >= 0) || (got == 0 && tc.want != 0) {
			t.Errorf("compareVersion(%q,%q) = %d want sign %d", tc.a, tc.b, got, tc.want)
		}
	}
}
