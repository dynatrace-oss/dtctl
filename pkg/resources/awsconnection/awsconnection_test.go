package awsconnection

import "testing"

func TestValidateRoleArn(t *testing.T) {
	cases := []struct {
		arn     string
		wantErr bool
	}{
		{"", false},
		{"arn:aws:iam::123456789012:role/DynatraceMonitoringRole", false},
		{"arn:aws:iam::123456789012:role/path/to/Role", false},
		{"arn:aws:iam::aws:role/ServiceRole", false},
		{"arn:aws:iam:::role/missing-account", true},
		{"not-an-arn", true},
		{"arn:aws:s3:::bucket", true},
	}
	for _, tc := range cases {
		err := ValidateRoleArn(tc.arn)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateRoleArn(%q) err=%v wantErr=%v", tc.arn, err, tc.wantErr)
		}
	}
}

func TestDynatraceAWSAccountID(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"https://abc12345.live.dynatrace.com", "314146291599"},
		{"https://abc12345.apps.dynatrace.com", "314146291599"},
		{"https://jvp48484.dev.apps.dynatracelabs.com", "476114158034"},
		{"https://abc.sprint.apps.dynatracelabs.com", "476114158034"},
		{"https://abc.live.dynatracelabs.com", "476114158034"},
		{"", "476114158034"},
	}
	for _, tc := range cases {
		got := DynatraceAWSAccountID(tc.baseURL)
		if got != tc.want {
			t.Errorf("DynatraceAWSAccountID(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}

func TestFlattenConnection(t *testing.T) {
	c := &AWSConnection{
		Value: Value{
			Name: "siwek",
			Type: TypeRoleBased,
			AwsRoleBasedAuthentication: &AwsRoleBasedAuthenticationConfig{
				RoleArn:   "arn:aws:iam::123456789012:role/Test",
				Consumers: []string{DefaultConsumer},
			},
		},
	}
	flattenConnection(c)
	if c.Name != "siwek" || c.Type != TypeRoleBased || c.RoleArn != "arn:aws:iam::123456789012:role/Test" {
		t.Errorf("flattenConnection produced unexpected fields: %+v", c)
	}
}
