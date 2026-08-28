---
title: Sessions
description: One conversation, fully recorded.
order: 2
---

<!-- PLACEHOLDER: scaffolding for the docs layout. Replace with real content. -->

A session is a single run of an agent against a task, stored as an append-only
transcript on disk. This page is a placeholder for the full reference.

## The transcript

Every message, tool call and result is written as it happens, so a crashed or
closed session can be resumed rather than restarted.

## Permission modes

A session runs read-only, ask-first or fully autonomous. The mode is the safety
boundary around whatever tools the agent holds.

## Cost tracking

Token usage and price are rolled up per session and per agent, so an expensive
run is visible while it is still running.
