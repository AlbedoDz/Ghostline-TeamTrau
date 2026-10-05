// Package zapret2 embeds the official zapret2 windows-x86_64 build (MIT),
// the Cygwin runtime it needs (LGPLv3) and the WinDivert driver it ships
// with (LGPLv3), all unmodified, plus the Lua libraries strategies call.
package zapret2

import "embed"

//go:embed winws2.exe cygwin1.dll WinDivert.dll WinDivert64.sys lua/zapret-lib.lua lua/zapret-antidpi.lua lua/zapret-auto.lua
var FS embed.FS
