---
title: Workspaces
description: The boundary around agents, data and files.
order: 3
---

<!-- PLACEHOLDER: scaffolding for the docs layout. Replace with real content. -->

A workspace groups agents, sessions, skills and board state around one root
directory. This page is a placeholder for the full reference.

## Isolation

Workspaces do not share state. Switching workspaces swaps the entire set of
agents and history along with the working directory.

## Shared instructions

Workspace-level instructions are prepended to every agent in it, which is where
project conventions belong.

## Storage layout

Everything lives in plain files under the workspace store, so it can be inspected,
backed up or deleted with ordinary tools.
