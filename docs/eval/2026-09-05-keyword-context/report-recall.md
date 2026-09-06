corpus eae65b6 · 2026-07-28 (stateless) · lexical only

찾았나(recall) · 실제로 받아 읽었나(delivery) · 그 대가로 삼킨 바이트

[base] 10세트
                                  graphin          grep 문장 그대로        grep 심볼 추측     
                             rec  del   bytes   rec  del   bytes   rec  del   bytes  
----------------------------------------------------------------------------------------
 truncate-cap                100% 100%      2K  100% 100%    185K  100% 100%    185K 
 inflight-wait               100% 100%      3K  100% 100%   7824K  100% 100%    656K 
 workspace-lock                0%  50%      4K  100% 100%   7327K   n/a    —       — 
 embed-crash-recovery          0%   0%      7K  100% 100%   7810K   n/a    —       — 
 versioning-rules              0%   0%     10K  100% 100%   6403K   n/a    —       — 
 graphindb-contract            0%   0%      4K  100% 100%   4085K   n/a    —       — 
 launcher-reinstall            0%   0%      2K  100% 100%   6883K   n/a    —       — 
 wordpiece                   100%   0%      3K  100% 100%     93K   n/a    —       — 
 watch-debounce              100%   0%      4K  100% 100%   4998K   n/a    —       — 
 rank-definition-vs-callers  100%   0%      8K  100% 100%   4269K   n/a    —       — 
----------------------------------------------------------------------------------------
  graphin           recall 50.0%  delivery 25%  46K  (위치 12K + 탐색 0B + 읽기 34K)
  grep 문장 그대로  recall 100.0%  delivery 100%  49878K
  grep 심볼 추측    recall 100.0%  delivery 100%  842K  · 추측 가능 2/10
  search_hybrid 힌트 발화: broad 8  (10문항)

[variants] 4세트
                                  graphin          grep 문장 그대로        grep 심볼 추측     
                             rec  del   bytes   rec  del   bytes   rec  del   bytes  
----------------------------------------------------------------------------------------
 workspace-lock-target         0%  50%      4K  100% 100%   7327K   n/a    —       — 
 embed-crash-recovery-target   0%   0%      4K  100% 100%   7810K   n/a    —       — 
 versioning-rules-target       0%   0%      9K  100% 100%   6403K   n/a    —       — 
 graphindb-contract-target   100%   0%      2K  100% 100%   4085K   n/a    —       — 
----------------------------------------------------------------------------------------
  graphin           recall 25.0%  delivery 12%  18K  (위치 5K + 탐색 0B + 읽기 13K)
  grep 문장 그대로  recall 100.0%  delivery 100%  25625K
  search_hybrid 힌트 발화: broad 4  (4문항)

[hop] 3세트
                                  graphin          grep 문장 그대로        grep 심볼 추측     
                             rec  del   bytes   rec  del   bytes   rec  del   bytes  
----------------------------------------------------------------------------------------
↳hop-truncate-callers        100% 100%      7K  100% 100%    185K  100% 100%    185K 
↳hop-bootstrap-background      0%   0%      7K  100% 100%   1324K  100% 100%   1324K 
↳hop-tokenize-callers          0%   0%      3K  100% 100%    766K  100% 100%    141K 
----------------------------------------------------------------------------------------
  graphin           recall 33.3%  delivery 33%  17K  (위치 3K + 탐색 2K + 읽기 11K)
  grep 문장 그대로  recall 100.0%  delivery 100%  2276K
  grep 심볼 추측    recall 100.0%  delivery 100%  1651K  · 추측 가능 3/3

[tests] 6세트
                                  graphin          grep 문장 그대로        grep 심볼 추측     
                             rec  del   bytes   rec  del   bytes   rec  del   bytes  
----------------------------------------------------------------------------------------
 test-drop-assert            100% 100%      5K  100% 100%   2254K  100% 100%     10K 
 test-breadcrumb-assert      100% 100%      4K  100% 100%   7668K  100% 100%      6K 
 test-rename-drift-assert    100% 100%      5K  100% 100%   6598K  100% 100%      8K 
 test-tier0-guard            100% 100%      3K  100% 100%   7374K   n/a    —       — 
 test-scan-edit-guard          0%   0%      9K  100% 100%   7803K   n/a    —       — 
 test-definition-rank-guard    0%   0%      8K  100% 100%   6224K   n/a    —       — 
----------------------------------------------------------------------------------------
  graphin           recall 66.7%  delivery 67%  34K  (위치 8K + 탐색 0B + 읽기 26K)
  grep 문장 그대로  recall 100.0%  delivery 100%  37923K
  grep 심볼 추측    recall 100.0%  delivery 100%  24K  · 추측 가능 3/6

  keyword 검색기 — 리터럴형 3문항, 골든 grep 문자열을 그대로 넣는다
                                   ctx=0              ctx=2              ctx=4       
                             rec  del   bytes   rec  del   bytes   rec  del   bytes  
 test-drop-assert            100% 100%    719B  100% 100%      1K  100% 100%      2K 
 test-breadcrumb-assert      100% 100%    457B  100% 100%    575B  100% 100%    790B 
 test-rename-drift-assert    100% 100%    639B  100% 100%      1K  100% 100%      1K 
  keyword ctx=0       recall 100.0%  delivery 100%  2K  · 접힌 파일 0 · id 없는 히트 0
  keyword ctx=2       recall 100.0%  delivery 100%  3K  · 접힌 파일 0 · id 없는 히트 0
  keyword ctx=4       recall 100.0%  delivery 100%  4K  · 접힌 파일 0 · id 없는 히트 0
  search_hybrid 힌트 발화: broad 5  (6문항)

scores → /tmp/claude-1000/-home-tipa-projects-graphin/36317fd5-e98f-486d-b61f-a974f7d060f8/scratchpad/recall-p1.json
