package cmd

import "testing"

func TestParseCloudPath(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		familyID string
		path     string
		wantErr  bool
	}{
		{name: "personal default", value: "/dir/file", path: "/dir/file"},
		{name: "personal prefix rejected", value: "personal:/dir/file", wantErr: true},
		{name: "family", value: "300002045464470:/dir/file", familyID: "300002045464470", path: "/dir/file"},
		{name: "family root short", value: "300002045464470:", familyID: "300002045464470", path: "/"},
		{name: "family relative rejected", value: "300002045464470:dir", wantErr: true},
		{name: "ordinary relative rejected", value: "dir/file", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			location, err := parseCloudPath(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr=%v", err, test.wantErr)
			}
			if err == nil && (location.familyID != test.familyID || location.path != test.path) {
				t.Fatalf("location = %#v, want family=%q path=%q", location, test.familyID, test.path)
			}
		})
	}
}

func TestCloudPathDisplay(t *testing.T) {
	location := cloudPath{familyID: "123", path: "/dir"}
	if got := location.display("/dir/file"); got != "123:/dir/file" {
		t.Fatalf("display = %q", got)
	}
}

func TestParseCloudPathWithDefaultFamily(t *testing.T) {
	tests := []struct {
		value    string
		familyID string
	}{
		{value: "/file", familyID: "123"},
		{value: "456:/file", familyID: "456"},
	}
	for _, test := range tests {
		location, err := parseCloudPathWithFamily(test.value, "123")
		if err != nil {
			t.Fatal(err)
		}
		if location.familyID != test.familyID || location.path != "/file" {
			t.Fatalf("value=%q location=%#v", test.value, location)
		}
	}
}

func TestCleanScopedCloudPath(t *testing.T) {
	if got := cleanCloudPath("123:/dir/../file"); got != "123:/file" {
		t.Fatalf("clean path = %q", got)
	}
}

func TestCloudPathNativeName(t *testing.T) {
	for _, test := range []struct{ value, want string }{
		{"/", "."},
		{"/dir/file", "dir/file"},
		{"123:/family/file", "family/file"},
	} {
		location, err := parseCloudPath(test.value)
		if err != nil {
			t.Fatal(err)
		}
		if got := location.name(); got != test.want {
			t.Fatalf("native name for %q = %q, want %q", test.value, got, test.want)
		}
	}
}
