---
title: Documentation
weight: 1
description: Drop operator documentation.
llmsDescription: |
  Documentation index for the drop Kubernetes operator. Sections: install,
  usage (CachedImage/CachedImageSet/PullPolicy examples), discovery
  (DiscoveryPolicy), monitoring (metrics/events), reference (CRD fields,
  errors, metrics, architecture), developing (build/test/contribute).
---

Drop pre-caches container images on Kubernetes nodes using short-lived Pods.

## Why

When many CI jobs or workloads start simultaneously, Kubernetes nodes face a thundering herd of image pulls. Concurrent pods on the same node all pulling the same large image saturate bandwidth, stall containerd, and cascade into failures.

| Problem | Impact |
|---------|--------|
| **Thundering herd** | Parallel pulls of the same image destabilize nodes |
| **Registry overload** | Sudden pull surges hit rate limits or cause outages |
| **Cold-start latency** | Large images delay workloads that need them immediately |

Drop pre-caches images *before* workloads need them, paces pulls to stay within safe limits, and automatically discovers which images matter most.

## Sections

| Section | What you'll find |
|---------|-----------------|
| [Installation](install/) | Helm install, prerequisites |
| [Usage](usage/) | CachedImage, CachedImageSet, PullPolicy examples |
| [Discovery](discovery/) | Automatic image discovery with DiscoveryPolicy |
| [Monitoring](monitoring/) | Prometheus metrics, events, status conditions |
| [Reference](reference/) | CRD field reference, errors, metrics, architecture |
| [Developing](developing/) | Build, test, lint, project structure |
| [For AI Agents](for-ai-agents/) | llms.txt, Markdown API, generation architecture |
