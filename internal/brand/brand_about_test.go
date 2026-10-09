package brand

import "testing"

func TestAbout(t *testing.T) {
	if Author != "TeamTrau" || RepoURL != "https://github.com/AlbedoDz/TeamTrau-Ghostline" {
		t.Fatalf("Author=%q RepoURL=%q", Author, RepoURL)
	}
}
