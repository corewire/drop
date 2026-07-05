# 20-run synthetic benchmark summary

This run used the synthetic GitLab Kubernetes executor generator with 25,000 jobs, 100 CI nodes and 30 OCI images.
The scenarios were generated with different seeds and then replayed under the evaluator policies.

Important caveat: this is synthetic data. It is meant to validate the evaluator workflow and show plausible behavior; it is not a measurement from a production cluster.

## Policy comparison

| Policy                  |   Mean affected jobs |   Mean affected job-minutes |   Stddev affected job-minutes |   Mean minutes avoided |   Mean % avoided |   Mean P95 wait seconds |   Mean P99 wait seconds |
|:------------------------|---------------------:|----------------------------:|------------------------------:|-----------------------:|-----------------:|------------------------:|------------------------:|
| No prewarming           |              1271.05 |                     1086.00 |                        104.87 |                   0.00 |             0.00 |                   31.92 |                   68.39 |
| Prewarm top 10 by usage |              1075.45 |                      914.20 |                        114.04 |                 171.80 |            15.82 |                    8.03 |                   64.01 |
| Prewarm top 30 by usage |                 0.00 |                        0.00 |                          0.00 |                1086.00 |           100.00 |                    0.00 |                    0.00 |
| Prewarm top 10 oracle   |               843.20 |                      548.75 |                         47.17 |                 537.25 |            49.47 |                    0.00 |                   45.58 |
| Spegel only             |              1267.75 |                      650.57 |                         62.72 |                 435.43 |            40.10 |                   19.15 |                   41.03 |
| Spegel + top 10         |              1073.30 |                      547.80 |                         68.26 |                 538.20 |            49.56 |                    4.82 |                   38.41 |

## Interpretation

- No prewarming created about 1086 affected job-minutes in the developer window on average.
- Prewarming the top 10 images by pre-09:00 usage avoided about 172 affected job-minutes (15.8%).
- Prewarming all 30 modeled images removed all modeled cold exposure in this synthetic setup. In a real cluster this would depend on pacing, timing, image GC and prewarm success rate.
- Spegel-only leaves the number of cold hits almost unchanged, but reduces affected job-minutes by reducing the modeled image availability time.
- Spegel plus top-10 prewarming performed similarly to the oracle top-10 impact policy in this synthetic run.
