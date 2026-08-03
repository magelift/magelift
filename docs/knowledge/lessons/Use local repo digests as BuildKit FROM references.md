---
type: lesson
title: Use local repo digests as BuildKit FROM references
description: A bare local Docker image ID such as sha256:<config> works with docker run but not as a Dockerfile
  FROM reference under the docker-container buildx driver; BuildKit interprets it as docker.io/libra...
tags:
- docker
- buildkit
- images
- e2e
generated:
  at: '2026-07-24'
---

A bare local Docker image ID such as sha256:<config> works with docker run but not as a Dockerfile FROM reference under the docker-container buildx driver; BuildKit interprets it as docker.io/library/sha256. Resolve the runtime image to a repository@sha256 repo digest and use the Docker-backed default builder for local loads so it can access the local image store. Keep the builder runner image as a bare ID because docker run accepts it.
