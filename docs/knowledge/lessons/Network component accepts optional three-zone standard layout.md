---
type: lesson
title: Network component accepts optional three-zone standard layout
description: The standard AWS network preset normally uses two zones, but the network component also accepts
  three zones when the production RabbitMQ cluster needs one subnet per broker.
tags:
- network
- rabbitmq
- aws
status: stable
generated:
  at: '2026-07-24'
---

The standard AWS network preset normally uses two zones, but the network component also accepts three zones when the production RabbitMQ cluster needs one subnet per broker. Target synthesis must choose this layout when queue capability minimum zones is three.
