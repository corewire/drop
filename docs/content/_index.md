---
title: Puller
layout: hextra-home
description: Kubernetes operator that pre-caches container images on cluster nodes.
llmsDescription: |
  Puller is a Kubernetes operator that pre-caches container images on cluster
  nodes. CachedImage CR → Puller Operator → Pod per node → kubelet pulls image
  → Pod exits → image cached. CRDs: CachedImage, CachedImageSet, PullPolicy,
  DiscoveryPolicy. API group puller.corewire.io/v1alpha1, all cluster-scoped.
  No privileged containers — uses kubelet image pulls only.
---

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Puller
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-8">
{{< hextra/hero-subtitle >}}
  Pre-cache container images on Kubernetes nodes.
{{< /hextra/hero-subtitle >}}
</div>

{{< tabs items="Apply + Status,Pods + Nodes,Events" >}}

{{< tab >}}
{{< asciinema file="casts/apply.cast" autoplay="true" loop="true" speed="0.75" >}}
{{< /tab >}}

{{< tab >}}
{{< asciinema file="casts/pods.cast" autoplay="true" loop="true" speed="0.75" >}}
{{< /tab >}}

{{< tab >}}
{{< asciinema file="casts/events.cast" autoplay="true" loop="true" speed="0.75" >}}
{{< /tab >}}

{{< /tabs >}}

> Create a CachedImage → operator spawns a Pod per node → kubelet pulls the image → Pod exits → image is warm. No privileges, no DaemonSets.

---

## I want to...

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Use Puller"
    subtitle="Install, create CachedImages, configure pacing and discovery."
    link="docs/install/"
  >}}
  {{< hextra/feature-card
    title="Develop Puller"
    subtitle="Architecture, CRD reference, build and test commands."
    link="docs/developing/"
  >}}
  {{< hextra/feature-card
    title="Feed to AI Agent"
    subtitle="llms.txt, Markdown API, full reference in one request."
    link="docs/for-ai-agents/"
  >}}
{{< /hextra/feature-grid >}}
