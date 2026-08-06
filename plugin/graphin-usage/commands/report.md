---
description: (deprecated) graphin 채택/폴백 리포트 — /graphin:report 를 대신 쓰라
---

이 명령은 **더 이상 유지되지 않는다.** 계측은 [`graphin`](../../graphin/README.md)
플러그인으로 합쳐졌다.

사용자에게 이렇게 안내하라:

1. `/plugin install graphin@graphin` — 서버·계측 훅·리포트가 한 플러그인에 있다.
2. 그다음부터는 **`/graphin:report`** 를 쓴다.
3. 이 플러그인(`graphin-usage`)은 제거해도 된다:
   `/plugin uninstall graphin-usage@graphin`.

**그동안 쌓인 이벤트는 그대로다.** 로그는 워크스페이스의
`.graphin/usage/events.jsonl`에 있고 두 플러그인이 같은 파일에 쓴다. 지금 당장
집계가 필요하면 프로젝트 루트에서 직접 실행하면 된다:

```sh
graphin usage report [--since 72h] [--json] [--top 30]
```
