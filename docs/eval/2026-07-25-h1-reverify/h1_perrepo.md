# H1 per-repo 층화 — hybrid − lexical (balanced 203, drop 0)


## k5-rrf60-mc75

| repo | n | Δrecall%p | Δrecall@300%p | Δndcg@300%p | ndcg 승/패 | ndcg p |
|---|--:|--:|--:|--:|:--:|--:|
| django/django | 25 | +1.08 | +1.95 | +4.57 | 12/11 | 1.000 |
| matplotlib/matplotlib | 25 | -0.25 | +0.15 | +0.81 | 12/12 | 1.000 |
| scikit-learn/scikit-learn | 25 | -3.81 | -0.22 | +7.16 | 15/9 | 0.307 |
| sphinx-doc/sphinx | 25 | -5.57 | -0.37 | +0.20 | 10/10 | 1.000 |
| sympy/sympy | 25 | +4.94 | +4.53 | +16.74 | 17/5 | 0.017 |
| astropy/astropy | 21 | +0.62 | +0.46 | +17.70 | 13/5 | 0.096 |
| pydata/xarray | 18 | +0.43 | +1.80 | +0.16 | 9/5 | 0.424 |
| pytest-dev/pytest | 18 | -1.43 | +0.93 | +8.58 | 11/3 | 0.057 |
| pylint-dev/pylint | 10 | -1.76 | +2.28 | +6.95 | 5/3 | 0.727 |
| psf/requests | 8 | +2.62 | -2.89 | -2.38 | 3/4 | 1.000 |
| mwaskom/seaborn | 2 | -5.55 | +4.48 | +28.39 | 2/0 | 0.500 |
| pallets/flask | 1 | +25.88 | +27.16 | +44.89 | 1/0 | 1.000 |

- ndcg@300 Δ평균이 양(+)인 repo: **11/12** (전반적이면 다양성 견고, 소수면 집중)

## k20-rrf60-mc75

| repo | n | Δrecall%p | Δrecall@300%p | Δndcg@300%p | ndcg 승/패 | ndcg p |
|---|--:|--:|--:|--:|:--:|--:|
| django/django | 25 | +3.41 | +3.38 | +7.81 | 15/8 | 0.210 |
| matplotlib/matplotlib | 25 | -1.22 | +2.21 | +3.80 | 13/10 | 0.678 |
| scikit-learn/scikit-learn | 25 | +2.40 | -0.16 | +11.15 | 15/7 | 0.134 |
| sphinx-doc/sphinx | 25 | -1.50 | +3.02 | +4.35 | 12/11 | 1.000 |
| sympy/sympy | 25 | +5.97 | +4.14 | +25.38 | 18/3 | 0.001 |
| astropy/astropy | 21 | +4.64 | +1.84 | +13.90 | 12/8 | 0.503 |
| pydata/xarray | 18 | +0.78 | -2.84 | -3.34 | 10/7 | 0.629 |
| pytest-dev/pytest | 18 | -0.12 | -0.64 | +7.00 | 10/5 | 0.302 |
| pylint-dev/pylint | 10 | +1.53 | +0.39 | -1.73 | 3/5 | 0.727 |
| psf/requests | 8 | -0.26 | -0.69 | -7.18 | 2/4 | 0.688 |
| mwaskom/seaborn | 2 | +0.48 | +3.61 | +18.55 | 2/0 | 0.500 |
| pallets/flask | 1 | +11.02 | +5.11 | +24.51 | 1/0 | 1.000 |

- ndcg@300 Δ평균이 양(+)인 repo: **9/12** (전반적이면 다양성 견고, 소수면 집중)
