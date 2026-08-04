/* graphin admin — 최소 클라이언트 스크립트.
   CSP가 default-src 'self'라 인라인 스크립트도 hx-on:*(내부적으로 eval)도
   쓸 수 없다(DECISIONS.md E3). 그래서 필요한 동작만 외부 파일로 둔다.
   문서 레벨 위임 하나로 처리하므로 htmx가 DOM을 교체해도 다시 붙일 필요가 없다. */
(function () {
  "use strict";

  var THEME_KEY = "graphin-admin-theme";

  function stored(key) {
    try {
      return window.localStorage.getItem(key);
    } catch (e) {
      return null; // 사생활 보호 모드 등 — 저장 없이 동작만 한다.
    }
  }

  function store(key, value) {
    try {
      window.localStorage.setItem(key, value);
    } catch (e) {
      /* 무시 */
    }
  }

  /* 테마: 저장값이 있으면 첫 페인트 전에 반영한다. 이 스크립트는 <head>에서
     동기로 로드되므로 깜빡임(FOUC)이 없다. */
  var saved = stored(THEME_KEY);
  if (saved === "light" || saved === "dark") {
    document.documentElement.setAttribute("data-theme", saved);
  }

  function currentTheme() {
    var attr = document.documentElement.getAttribute("data-theme");
    if (attr === "light" || attr === "dark") {
      return attr;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  function syncToggle(btn, theme) {
    // 색만으로 상태를 전달하지 않는다(DESIGN.md §9) — 레이블도 함께 바꾼다.
    var next = theme === "dark" ? "밝은" : "어두운";
    btn.setAttribute("aria-label", next + " 테마로 전환");
    btn.setAttribute("aria-pressed", theme === "dark" ? "true" : "false");
    btn.textContent = theme === "dark" ? "☀" : "☾";
  }

  function toggleTheme(btn) {
    var next = currentTheme() === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    store(THEME_KEY, next);
    syncToggle(btn, next);
  }

  document.addEventListener("DOMContentLoaded", function () {
    var btn = document.querySelector("[data-theme-toggle]");
    if (btn) {
      syncToggle(btn, currentTheme());
    }
  });

  document.addEventListener("click", function (ev) {
    var target = ev.target;
    if (!target || !target.closest) {
      return;
    }

    var toggle = target.closest("[data-theme-toggle]");
    if (toggle) {
      toggleTheme(toggle);
      return;
    }

    /* 트리 캐럿(DESIGN.md §4.4). 첫 클릭은 htmx가 자식을 가져오고(once),
       이후 클릭은 여기서 접기/펼치기만 한다 — 다시 요청하면 중복된다. */
    var caret = target.closest(".caret");
    if (caret && !caret.classList.contains("leaf")) {
      var li = caret.closest("li");
      var collapsed = li.classList.toggle("collapsed");
      caret.textContent = collapsed ? "▸" : "▾";
      caret.setAttribute("aria-expanded", collapsed ? "false" : "true");
      li.setAttribute("aria-expanded", collapsed ? "false" : "true");
      return;
    }

    /* 코드 복사(DESIGN.md §4.5 헤더 바). 행 번호 열은 빼고 소스만 모은다.
       localhost는 secure context라 navigator.clipboard를 쓸 수 있다. */
    var copyBtn = target.closest("[data-copy-code]");
    if (copyBtn) {
      var block = copyBtn.closest(".code");
      var lines = block ? block.querySelectorAll(".src") : [];
      var text = Array.prototype.map
        .call(lines, function (el) {
          return el.textContent;
        })
        .join("\n");
      var done = function (label) {
        copyBtn.textContent = label;
        window.setTimeout(function () {
          copyBtn.textContent = "복사";
        }, 1200);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(
          function () {
            done("복사됨");
          },
          function () {
            done("실패");
          }
        );
      } else {
        done("실패");
      }
      return;
    }

    /* 용어 도움말 팝오버(DESIGN.md §4.3): 트리거는 클릭뿐이고, 바깥을
       누르면 닫힌다. 내용은 htmx가 .popover 안에 넣는다. */
    var help = target.closest(".help");
    document.querySelectorAll(".popover").forEach(function (pop) {
      var ownTrigger = help && help.parentElement === pop.parentElement;
      if (ownTrigger) {
        return; // 자기 팝오버는 htmx 응답이 채운다.
      }
      if (!target.closest(".popover")) {
        pop.innerHTML = "";
        var owner = pop.parentElement.querySelector(".help");
        if (owner) {
          owner.setAttribute("aria-expanded", "false");
        }
      }
    });

    if (help) {
      help.setAttribute("aria-expanded", "true");
    }
  });

  document.addEventListener("keydown", function (ev) {
    if (ev.key !== "Escape") {
      return;
    }
    document.querySelectorAll(".popover").forEach(function (pop) {
      pop.innerHTML = "";
      var owner = pop.parentElement.querySelector(".help");
      if (owner) {
        owner.setAttribute("aria-expanded", "false");
      }
    });
  });
})();
