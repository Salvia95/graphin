package provision

// Recorded from the pinned artifacts on 2026-07-21 (§8: ORT-모델 호환성
// 튜플 고정). Regenerate only when deliberately bumping the tuple.
const (
	ortLinuxAMD64SHA256 = "1254da24fb389cf39dc0ff3451ab48301740ffbfcbaf646849df92f80ee92c57"
	e5ModelSHA256       = "7d9092cb25f2bd1c023b7e8d2aa459044a02030ac880e5a59fdaf27af69f1ded"
	me5ModelSHA256      = "f80102d3f2a1229f387d3c81909990d8945513e347b0eab049f7de3c6f98c193"
)

// Recorded on 2026-08-05 when linux/arm64 joined the release matrix
// (docs/plugin-distribution.md D2). Same ORT release, same .so name as x64.
const ortLinuxARM64SHA256 = "34ff1c2d0f12e2cf3d33a0c5f82e39792e1d581fbd6968fd7c30d173654be01a"

// Recorded on 2026-08-06 (v1.1). Apple Silicon only — the 1.26.0 release
// publishes no osx-x86_64 asset, so darwin/amd64 stays unsupported.
const ortDarwinARM64SHA256 = "7a1280bbb1701ea514f71828765237e7896e0f2e1cd332f1f70dbd5c3e33aca3"
