# Third-Party Notices

TionHarness is licensed under the Apache License, Version 2.0 (see `LICENSE`).

The components listed below are third-party software that is either embedded in
the distributed TionHarness binary or otherwise redistributed with it. Each
component remains under its own license and copyright. Where a license requires
its full text or a copyright notice to be reproduced, that text is included
verbatim in this file.

---

## Go dependencies

| Component | License |
|-----------|---------|
| `github.com/google/uuid` | BSD-3-Clause |
| `golang.org/x/sys` | BSD-3-Clause |
| `github.com/jchv/go-webview2` | MIT |
| `github.com/jchv/go-winloader` | ISC |
| `github.com/robfig/cron/v3` | MIT |

### Microsoft Edge WebView2 Loader SDK

The `github.com/jchv/go-webview2` module vendors the Microsoft Edge WebView2
Loader SDK under `webviewloader/sdk/`. Its license requires the copyright notice
and the conditions below to be reproduced in binary redistributions, which
includes the distributed TionHarness desktop binary.

```
Copyright (C) Microsoft Corporation. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * The name of Microsoft Corporation, or the names of its contributors
may not be used to endorse or promote products derived from this
software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## Frontend runtime dependencies (embedded in the binary)

The built frontend assets are embedded into the TionHarness binary, so the
runtime dependencies below are redistributed with it.

| Component | License | Notes |
|-----------|---------|-------|
| `react`, `react-dom` | MIT | Copyright (c) Meta Platforms, Inc. and affiliates |
| `mermaid` | MIT | |
| `@xyflow/react` | MIT | |
| `i18next`, `react-i18next` | MIT | |
| `lucide-react` | ISC | A subset of icons is derived from Feather (MIT) — see below |
| `highlight.js` | BSD-3-Clause | Full text below |
| `vis-network` | Apache-2.0 OR MIT | |
| `dompurify` | MPL-2.0 OR Apache-2.0 | TionHarness elects the **Apache-2.0** branch of this dual license |
| `simple-icons` | CC0-1.0 | Icon set only; brand names and logos remain the property of their owners — see the trademark note in `NOTICE` |
| `khroma` | MIT | |

### lucide-react

`lucide-react` is distributed under the ISC License:

```
Copyright (c) 2026 Lucide Icons and Contributors
```

A subset of the Lucide icons is derived from the Feather project and carries the
MIT License:

```
The MIT License (MIT)

Copyright (c) 2013-present Cole Bemis
```

### highlight.js

```
BSD 3-Clause License

Copyright (c) 2006, Ivan Sagalaev.
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.

* Neither the name of the copyright holder nor the names of its
  contributors may be used to endorse or promote products derived from
  this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## Fonts

The following variable fonts are bundled with the frontend and embedded in the
binary. Both are licensed under the SIL Open Font License, Version 1.1.

| Font | Package | Copyright |
|------|---------|-----------|
| Inter Variable | `@fontsource-variable/inter` | Copyright 2016 The Inter Project Authors (https://github.com/rsms/inter) |
| JetBrains Mono Variable | `@fontsource-variable/jetbrains-mono` | Copyright 2020 The JetBrains Mono Project Authors (https://github.com/JetBrains/JetBrainsMono) |

### SIL Open Font License, Version 1.1

```
-----------------------------------------------------------
SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007
-----------------------------------------------------------

PREAMBLE
The goals of the Open Font License (OFL) are to stimulate worldwide
development of collaborative font projects, to support the font creation
efforts of academic and linguistic communities, and to provide a free and
open framework in which fonts may be shared and improved in partnership
with others.

The OFL allows the licensed fonts to be used, studied, modified and
redistributed freely as long as they are not sold by themselves. The
fonts, including any derivative works, can be bundled, embedded,
redistributed and/or sold with any software provided that any reserved
names are not used by derivative works. The fonts and derivatives,
however, cannot be released under any other type of license. The
requirement for fonts to remain under this license does not apply
to any document created using the fonts or their derivatives.

DEFINITIONS
"Font Software" refers to the set of files released by the Copyright
Holder(s) under this license and clearly marked as such. This may
include source files, build scripts and documentation.

"Reserved Font Name" refers to any names specified as such after the
copyright statement(s).

"Original Version" refers to the collection of Font Software components as
distributed by the Copyright Holder(s).

"Modified Version" refers to any derivative made by adding to, deleting,
or substituting -- in part or in whole -- any of the components of the
Original Version, by changing formats or by porting the Font Software to a
new environment.

"Author" refers to any designer, engineer, programmer, technical
writer or other person who contributed to the Font Software.

PERMISSION & CONDITIONS
Permission is hereby granted, free of charge, to any person obtaining
a copy of the Font Software, to use, study, copy, merge, embed, modify,
redistribute, and sell modified and unmodified copies of the Font
Software, subject to the following conditions:

1) Neither the Font Software nor any of its individual components,
in Original or Modified Versions, may be sold by itself.

2) Original or Modified Versions of the Font Software may be bundled,
redistributed and/or sold with any software, provided that each copy
contains the above copyright notice and this license. These can be
included either as stand-alone text files, human-readable headers or
in the appropriate machine-readable metadata fields within text or
binary files as long as those fields can be easily viewed by the user.

3) No Modified Version of the Font Software may use the Reserved Font
Name(s) unless explicit written permission is granted by the corresponding
Copyright Holder. This restriction only applies to the primary font name as
presented to the users.

4) The name(s) of the Copyright Holder(s) or the Author(s) of the Font
Software shall not be used to promote, endorse or advertise any
Modified Version, except to acknowledge the contribution(s) of the
Copyright Holder(s) and the Author(s) or with their explicit written
permission.

5) The Font Software, modified or unmodified, in part or in whole,
must be distributed entirely under this license, and must not be
distributed under any other license. The requirement for fonts to
remain under this license does not apply to any document created
using the Font Software.

TERMINATION
This license becomes null and void if any of the above conditions are
not met.

DISCLAIMER
THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO ANY WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT
OF COPYRIGHT, PATENT, TRADEMARK, OR OTHER RIGHT. IN NO EVENT SHALL THE
COPYRIGHT HOLDER BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
INCLUDING ANY GENERAL, SPECIAL, INDIRECT, INCIDENTAL, OR CONSEQUENTIAL
DAMAGES, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF THE USE OR INABILITY TO USE THE FONT SOFTWARE OR FROM
OTHER DEALINGS IN THE FONT SOFTWARE.
```

---

## Build-time only (not distributed)

The following packages are used only while building and are not shipped as code
in the distributed TionHarness binary or in the published static site:

- `sharp` and the `@img/sharp-libvips-*` platform binaries (LGPL-3.0-or-later)
  and `lightningcss` (MPL-2.0) are part of the `website/` Astro build chain. They
  run at build time to produce images and CSS; no code from them is linked into
  the TionHarness binary or emitted into the published site output.
- `caniuse-lite` (CC-BY-4.0) is a development-only dependency used for browser
  target data.

---

## Prior art and inspiration (no code copied)

TionHarness is an independent implementation, but several of its design ideas
were shaped by reading other agent projects. No source code from these projects
is copied into or distributed with TionHarness; they are listed here to credit
the prior art honestly.

- **Craft Agent OSS** (`craft-ai-agents/craft-agents-oss`) — informed the session
  self-management prompt section, the environment marker, the working-directory
  context block, and the shift-tab permission-cycle interaction. TionHarness
  deliberately diverges on prompt strategy (a lean prefix that delegates to
  skills, rather than a large static system prompt).
- **GitHub Copilot CLI** — its `/chronicle` session-insight command family was
  read as a reference when shaping TionHarness' own retrospective scanning.
- **Hermes Agent** — informed the error-classifier failover pattern, the
  tool-loop guardrail thresholds, the frozen prompt-snapshot ("prompt epoch")
  idea, and the verify-after-mutation check.

Each of these influences is a design pattern or interaction idea, independently
implemented in Go and TypeScript for this codebase.
