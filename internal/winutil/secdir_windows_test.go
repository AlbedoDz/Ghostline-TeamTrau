package winutil

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestAdminOnlySDDL(t *testing.T) {
	sd, err := windows.SecurityDescriptorFromString(AdminOnlySDDL)
	require.NoError(t, err)
	s := sd.String()
	require.True(t, strings.HasPrefix(s, "D:P"), s) // protected: nothing inherited from %ProgramData%
	require.Contains(t, s, ";;;SY)")
	require.Contains(t, s, ";;;BA)")
	require.NotContains(t, s, ";;;BU)")
	require.NotContains(t, s, ";;;AU)")
}

func TestOwnedByAdmins_UserFileIsNot(t *testing.T) {
	p := t.TempDir() + `\f.txt`
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	ok, err := OwnedByAdmins(p)
	require.NoError(t, err)
	if IsAdmin() {
		t.Skip("elevated test run: files are owned by Administrators")
	}
	require.False(t, ok, "a file created by a normal user must not count as admin-owned")
}
