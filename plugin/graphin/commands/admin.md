---
description: graphin 관리자 페이지 켜기 — 이 프로젝트 전용 주소를 지정하고 여는 법을 안내한다
allowed-tools: Bash(*), Read, Write
---

이 프로젝트에서 graphin **관리자 페이지**(읽기 전용 로컬 웹 UI)를 켜라.

## 왜 파일로 지정하는가

플러그인 옵션 `admin_addr`는 user settings에만 저장되는 **전역 값 하나**다. 여러
프로젝트에서 동시에 graphin을 쓰면 전부 같은 포트를 노려 뒤에 뜬 쪽이
`bind: address already in use`로 죽는다. 그래서 프로젝트별 주소는 파일로 준다:

```sh
mkdir -p .graphin
echo "127.0.0.1:7466" > .graphin/admin-addr
```

이 파일이 있으면 런처가 전역 `admin_addr`보다 **먼저** 읽는다.

## 절차

1. 현재 상태를 확인한다.

   ```sh
   cat .graphin/admin-addr 2>/dev/null
   ss -ltn 2>/dev/null | grep -E '7466|7467|7468' || true
   ```

2. 포트를 고른다. 사용자가 지정하지 않았으면 `127.0.0.1:7466`부터 시작하되,
   **이미 점유된 포트는 피하라.** 반드시 루프백(`127.0.0.1`/`localhost`)이어야 한다 —
   서버가 그 외 주소는 거부한다.

3. 파일에 쓴다(위 명령).

4. **주소는 서버 기동 시점에만 읽힌다.** `/mcp`에서 graphin을 재연결하거나 세션을
   새로 시작해야 적용된다고 안내하고, 그다음 `http://<주소>`를 열라고 말하라.

## 끄기

```sh
rm -f .graphin/admin-addr
```

전역 `admin_addr`도 비어 있으면 페이지는 뜨지 않는다(기본값). 포트를 자동으로
할당하지 않는 것은 의도다 — 조용한 포트 드리프트가 눈에 보이는 바인드 실패보다 나쁘다.
