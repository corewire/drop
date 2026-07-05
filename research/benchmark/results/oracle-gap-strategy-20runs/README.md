# Oracle-gap discovery strategy benchmark results

Synthetic 20-run benchmark for discovery strategies that try to close the gap to the oracle impact ranking.

Scenario:
- 20 independently generated synthetic days
- 25,000 GitLab Kubernetes executor jobs per evaluation day
- 100 CI nodes
- 30 OCI images
- Separate historical discovery day and evaluation day per run
- Shared image profile between discovery and evaluation day
- Top-10 images prewarmed for each strategy
- Prewarming is modeled as selected images being available at rotation/start, to isolate ranking quality

Strategies:
- count
- dev_weighted_count
- recent_count
- peak_concurrency
- hybrid_count_concurrency_a0.5
- count_x_pull_time
- dev_weighted_count_x_pull_time
- model_exposure_dev
- oracle_impact_upper_bound

The oracle is an after-the-fact developer-window impact ranking and is not deployable.
