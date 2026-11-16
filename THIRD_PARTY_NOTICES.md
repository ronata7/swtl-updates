# Third-party notices

## Included in the built programs
SWTLUpdater.exe and swtlpublish.exe are written in Go and contain parts of the Go standard library and runtime:

    Copyright 2009 The Go Authors.
    
    Redistribution and use in source and binary forms, with or without
    modification, are permitted provided that the following conditions are
    met:
    
       * Redistributions of source code must retain the above copyright
    notice, this list of conditions and the following disclaimer.
       * Redistributions in binary form must reproduce the above
    copyright notice, this list of conditions and the following disclaimer
    in the documentation and/or other materials provided with the
    distribution.
       * Neither the name of Google LLC nor the names of its
    contributors may be used to endorse or promote products derived from
    this software without specific prior written permission.
    
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

## NOT in this repository - used from the Tools folder
These belong to their authors and are not hosted or redistributed in this repository or by online updates. A player zip handed out separately may contain them, unmodified, under their own terms:
- SWAdmin - GreenDamTan, https://github.com/GreenDamTan/swadmin (the repository has no licence file); its command-line version (SWAdmin.Cli) is credited to rockon, not independently confirmed; bundles Newtonsoft.Json by James Newton-King, MIT. Only needed if `Converter=swadmin`; not used by default.
- SW Res structs dumper - Asaduji, https://github.com/Asaduji/SW-Res-structs-dumper (shown as MIT-licensed on its GitHub page; if you bundle it, keep its LICENSE.txt next to it; bundles Zydis by zyantific)
- 7-Zip (7z.exe, 7z.dll) - Igor Pavlov. Used unmodified. 7-Zip is licensed under the GNU LGPL; its source code is at https://www.7-zip.org

## Credit for a replaced tool
The XOR step of the data file (data12.v) used to be done with Xor by Luigi Auriemma (https://aluigi.org). It is now built into the updater (`xor.go`, independent code), and xor.exe is no longer needed or used. Thanks to the author.

The schema step (`schemagen.go`) and the CSV-to-table conversion (`resconv.go`) are now done by the updater itself by default (setting `Converter=builtin`). They are independent code written from the file formats as seen in sample files, and are checked against SWAdmin's output (the schema for all 356 tables; the tables byte for byte on test files). SWAdmin remains credited and is still the default until this has been confirmed on real data.
