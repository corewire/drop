---
title: Developer Guide
weight: 6
description: Everything you need to build, debug, test, and extend Puller.
llmsDescription: |
  Developer guide index. Links to architecture, local dev setup, build commands,
  testing, debugging, extending (new CRDs), code conventions, and release process.
---

This guide covers everything needed to work on Puller — from first checkout to shipping a release.

{{< cards >}}
  {{< card link="developing/architecture" title="Architecture" subtitle="Package graph, reconciler flows, design decisions" >}}
  {{< card link="developing/setup" title="Local Dev Setup" subtitle="Prerequisites, kind cluster, Tilt" >}}
  {{< card link="developing/testing" title="Testing" subtitle="envtest, Chainsaw e2e, patterns" >}}
  {{< card link="developing/debugging" title="Debugging" subtitle="Logs, common issues, pacing diagnostics, Delve" >}}
  {{< card link="developing/extending" title="Extending" subtitle="Adding a new CRD step-by-step" >}}
  {{< card link="developing/conventions" title="Conventions" subtitle="Naming, status patterns, import order, don'ts" >}}
  {{< card link="developing/releasing" title="Releasing" subtitle="Tag-triggered CI, multi-arch builds, Helm OCI" >}}
{{< /cards >}}
