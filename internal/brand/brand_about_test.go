package brand

import "testing"

func TestAbout(t *testing.T) {
	if Author != "Harry Nguyen" || RepoURL != "https://github.com/hashcott/ghostline" {
		t.Fatalf("Author=%q RepoURL=%q", Author, RepoURL)
	}
}
