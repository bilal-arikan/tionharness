---
title: Configuration
description: Settings, their defaults and where they live.
order: 1
---

<!-- PLACEHOLDER: scaffolding for the docs layout. Replace with real content. -->

Application-wide settings live in a single JSON file next to the store. This page
is a placeholder for the generated settings table.

## Settings file

The file is read at startup and written back whenever a value changes in the UI,
so hand edits and UI edits use the same format.

| Key | Default | Purpose |
| --- | --- | --- |
| `port` | `8080` | HTTP port the control plane listens on |
| `theme` | `dark` | Default interface theme |

## Environment overrides

A handful of values can be overridden by environment variables for containerised
or scripted runs.

## Provider credentials

Credentials are stored separately from settings and are never included in an
exported workspace.
