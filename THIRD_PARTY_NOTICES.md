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
- SWAdmin tools - rockon (bundles Newtonsoft.Json by James Newton-King, MIT)
- SW Res structs dumper - Asaduji, https://github.com/Asaduji/SW-Res-structs-dumper (MIT per its README; bundles Zydis by zyantific)
- 7-Zip (7z.exe, 7z.dll) - Igor Pavlov. Used unmodified. 7-Zip is licensed under the GNU LGPL; its source code is at https://www.7-zip.org

## Credit for a replaced tool
The XOR step of the data file (data12.v) used to be done with Xor by Luigi Auriemma (https://aluigi.org). It is now built into the updater (`xor.go`, independent code), and xor.exe is no longer needed or used. Thanks to the author.

The CSV-to-table conversion (`resconv.go`) can also be done by the updater itself (setting `Converter`). It is independent code written from the table format as seen in sample files, and it is checked against SWAdmin's output byte for byte. SWAdmin is still credited and still used for the schema step.
