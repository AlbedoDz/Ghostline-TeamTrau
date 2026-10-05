package zapret2

// Version is the bundled zapret2 release.
const Version = "v1.0.5.2"

// Pinned holds the SHA-256 of the zapret2 files Ghostline bundles: the
// windows-x86_64 binaries (as listed in the release's sha256sum.txt) and the
// three Lua libraries the strategies call into.
var Pinned = map[string]string{
	"winws2.exe":             "eb9807972c15f05a00416b0549d766a71c9186f8101d406e3cc6c16278164d9e",
	"cygwin1.dll":            "103104a52e5293ce418944725df19e2bf81ad9269b9a120d71d39028e821499b",
	"WinDivert.dll":          "16abd6a029e65557c6a309bea7b13bf81fff4e193567582e1cddbf6719f323e0",
	"WinDivert64.sys":        "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2",
	"lua/zapret-lib.lua":     "b67a470f23b00a8d6e732c4e5135a39b224511e0b71809d5f4616adf62674980",
	"lua/zapret-antidpi.lua": "31c9dd75b0bd55e98e5306293f2be81e9d2ecadcbbf9157394ff37dcff7dc85a",
	"lua/zapret-auto.lua":    "c2028f544134b0dec33f054ac54fe4f2315d969ba034601456f72017fd5f8528",
}
