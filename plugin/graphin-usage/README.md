# graphin-usage — 폐기됨 (묘비)

계측은 **[`graphin`](../graphin/README.md) 플러그인으로 합쳐졌다.** 서버와 훅이
한 플러그인에 있어야 워크스페이스 설정(`workspace_subdir`)을 공유하고, 훅이
플러그인이 설치한 바이너리를 곧장 찾을 수 있다.

```
/plugin install graphin@graphin
/plugin uninstall graphin-usage@graphin
```

## 0.2.0에서 무엇이 바뀌었나

**`PostToolUse` 훅이 제거됐다.** 이 플러그인은 이제 아무것도 기록하지 않는다.
`graphin` 플러그인의 훅이 그 일을 한다.

0.1.1에 머무르는 설치는 **계속 동작한다.** 두 플러그인이 함께 설치돼 훅이 두 번
발화해도 지표는 왜곡되지 않는다 — `internal/usage/stream.go`가 읽는 시점에
`tool_use_id`로 디듀프하므로 디스크만 두 배로 쓸 뿐이다. 그래서 서두를 필요 없이
갱신하면 된다.

`/graphin-usage:report`는 `/graphin:report`를 가리키는 안내로 바뀌었다.

## 쌓인 데이터는 어떻게 되나

그대로 있다. 두 플러그인이 같은 파일(`<workspace>/.graphin/usage/events.jsonl`)에
쓰므로 이관할 것이 없다. `graphin usage report`가 예전 이벤트와 새 이벤트를 함께
집계한다.

## 문서

- 계측 설계: [`docs/usage-spec.md`](../../docs/usage-spec.md)
- 프라이버시(무엇을 기록하고 무엇을 기록하지 않는가)·트러블슈팅:
  [`plugin/graphin/README.md`](../graphin/README.md)
- 진단: `/graphin:doctor`
