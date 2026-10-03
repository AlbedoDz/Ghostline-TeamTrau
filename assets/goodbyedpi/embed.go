// Package goodbyedpi embeds the official GoodbyeDPI 0.2.2 x86_64 build
// (Apache-2.0) and the WinDivert driver it ships with (LGPLv3).
package goodbyedpi

import "embed"

//go:embed goodbyedpi.exe WinDivert.dll WinDivert64.sys
var FS embed.FS
