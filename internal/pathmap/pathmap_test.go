package pathmap

import "testing"

func TestTranslatePath(t *testing.T) {
	tests := []struct {
		name       string
		localRoot  string
		remoteRoot string
		localPath  string
		want       string
		wantErr    bool
	}{
		{
			name:       "windows root, real-world example",
			localRoot:  `G:\Comics\`,
			remoteRoot: "/data",
			localPath:  `G:\Comics\12-Gauge Comics\Sherwood, TX (2014 Limited Series)\Sherwood, TX Vol.2014 #01 (of 05) (July, 2014).cbz`,
			want:       "/data/12-Gauge Comics/Sherwood, TX (2014 Limited Series)/Sherwood, TX Vol.2014 #01 (of 05) (July, 2014).cbz",
		},
		{
			name:       "root without trailing separator",
			localRoot:  `G:\Comics`,
			remoteRoot: "/data",
			localPath:  `G:\Comics\Batman\Batman #1.cbz`,
			want:       "/data/Batman/Batman #1.cbz",
		},
		{
			name:       "remote root with trailing slash",
			localRoot:  `G:\Comics\`,
			remoteRoot: "/data/",
			localPath:  `G:\Comics\Batman\Batman #1.cbz`,
			want:       "/data/Batman/Batman #1.cbz",
		},
		{
			name:       "case-insensitive root match",
			localRoot:  `g:\comics\`,
			remoteRoot: "/data",
			localPath:  `G:\Comics\Batman\Batman #1.cbz`,
			want:       "/data/Batman/Batman #1.cbz",
		},
		{
			name:       "already forward-slash path",
			localRoot:  "/mnt/comics",
			remoteRoot: "/data",
			localPath:  "/mnt/comics/Batman/Batman #1.cbz",
			want:       "/data/Batman/Batman #1.cbz",
		},
		{
			name:       "path not rooted at local_root",
			localRoot:  `G:\Comics\`,
			remoteRoot: "/data",
			localPath:  `D:\Other\Batman #1.cbz`,
			wantErr:    true,
		},
		{
			name:       "path shorter than root",
			localRoot:  `G:\Comics\Long\Root\`,
			remoteRoot: "/data",
			localPath:  `G:\Short.cbz`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TranslatePath(tt.localRoot, tt.remoteRoot, tt.localPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("TranslatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("TranslatePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolve_TranslatesWhenBothRootsConfigured(t *testing.T) {
	got, ok := Resolve(`G:\Comics\`, "/data", `G:\Comics\Batman\Batman #1.cbz`)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if want := "/data/Batman/Batman #1.cbz"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolve_FalseWhenRootsNotConfigured(t *testing.T) {
	cases := []struct {
		name       string
		localRoot  string
		remoteRoot string
	}{
		{"both empty", "", ""},
		{"only localRoot", `G:\Comics\`, ""},
		{"only remoteRoot", "", "/data"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := Resolve(tt.localRoot, tt.remoteRoot, `G:\Comics\Batman\Batman #1.cbz`); ok {
				t.Error("expected ok=false when a root is unconfigured")
			}
		})
	}
}

func TestResolve_FalseWhenPathNotRooted(t *testing.T) {
	if _, ok := Resolve(`G:\Comics\`, "/data", `D:\Other\Batman #1.cbz`); ok {
		t.Error("expected ok=false when path isn't rooted at localRoot")
	}
}
