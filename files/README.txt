SW TL Updater 2.6.1  (by ronata)
==============================================================================
Patches SoulWorker's data12.v with your translated .csv files.
Uses the same tools as before (the Tools folder) - nothing there is replaced.

Folder layout (everything next to SWTLUpdater.exe)
------------------------------------------------------------------------------
  SWTLUpdater.exe
  Tools\        7z, xor, SWAdmin, SW Res structs dumper (not included in downloads - see "Credits"; the program
                tells you which files it needs and where to get them)
  TL csv\       your translated .csv files
Created automatically:
  SWTLUpdater.ini   your settings          backup\   previous patched files
  logs\             one log per run        cache\    saved table schema (speeds things up)
  data12.v          the newest patched file

How to use
------------------------------------------------------------------------------
 1. Close SoulWorker, and close SWPatcher or make sure it is idle (not waiting for the client).
    (The updater warns you if either one is running, and only one copy of the updater can run at a time.)
 2. Double-click SWTLUpdater.exe.
 3. First time: the starting settings are shown. Press Enter to accept them,
    or type C to change them (a folder window opens when you press Enter at a folder question).
 4. Choose 1) Patch now. Wait for "Patching complete".
    Not sure about something? 2) Dry run does everything except save files.

Menu
------------------------------------------------------------------------------
  1) Patch now        2) Dry run        3) Settings
  4) Restore a backup (puts a previous patched file back into the patcher folder)
  5) Open logs folder                   6) About, credits & help
  7) Check for updates                  0) Exit

Settings and their default values
------------------------------------------------------------------------------
Change them in the menu (3) Settings) or by editing SWTLUpdater.ini with Notepad.

  Setting (in the menu)       Key in the .ini      Default value
  --------------------------  -------------------  ---------------------------------------------
  Game folder                 GameFolder           C:\Wemade Online\Soulworker
  Translation folder          TranslationFolder    <program folder>\TL csv
  Patcher folder              PatcherFolder        %LocalAppData%\SWPatcher\jp\English\datas
  Game data file to patch     SourceDataFile       data12            (read from <game>\datas\data12.v)
  Name of the patched file    OutputDataFile       data12
  Backups to keep             BackupsToKeep        5                 (0 = keep all)
  Tables to skip              SkipTables           tb_Loading_String,tb_Option_String,tb_Talk_String
  Ask about settings at start AskAtStartup         yes               (yes / no)
  Update address              UpdateURL            (empty = online updates off; comes with your download)
  Update key                  UpdateKey            (empty; comes with your download - don't change it)
  Check for updates at start  UpdateAtStartup      yes               (yes / no)

  - "Reset everything to the default values" in the Settings menu restores the table above.
  - The old .bat's Readme.txt / rSetting.ini are no longer read - the program asks for the folders
    the first time and remembers them in SWTLUpdater.ini.

Optional command-line switches (for shortcuts or scripts; no menu is shown)
------------------------------------------------------------------------------
  --run           patch now              --dry-run     check everything, save nothing
  --restore       restore newest backup  --strict      CSV problems stop the run
  --keep-work     keep the temp folder   --settings    open the settings menu
  --update        check for updates and install them (add --run to patch afterwards)
  --appdir PATH   use another folder for Tools / TL csv / settings
  Exit codes: 0 ok, 1 error, 3 nothing changed.

What it checks for you
------------------------------------------------------------------------------
  - Every tool step; a failure stops the run with a clear message.
  - Your CSVs: valid UTF-8 and the same number of columns on every row.
  - The rebuilt data12 must contain exactly the original files (none lost, none duplicated),
    tables are stored like the originals (ZipCrypto + Deflate), and the whole archive is
    extracted and compared before anything is saved.
  - The game (SoulWorker100.exe) and SWPatcher are not running, and no second updater is open.
  - The previous patcher file is backed up before it is replaced; results are saved safely
    (temp file + rename), and every run writes a log.

Troubleshooting
------------------------------------------------------------------------------
  - "Tools folder is incomplete": copy the Tools folder next to the exe.
  - "SoulWorker not found": Settings > Game folder.
  - Problems? Send the newest file from the logs folder (menu 5) to Discord: ronata
  - Needs 64-bit Windows 10 or newer. Windows may warn about an "unknown publisher"
    because the exe is not code-signed: More info > Run anyway.

Online updates
------------------------------------------------------------------------------
  If your download came with an update address and key, the program checks for new versions of itself,
  the Tools and the translation files when it starts (or press 7). It asks before installing, verifies a
  digital signature on everything, and only touches SWTLUpdater.exe, README.txt, Tools\ and TL csv\.
  Turn it off in Settings (Update address: -).

Help / contact
------------------------------------------------------------------------------
  Any question or issue:  Discord: ronata   (attach the newest file from the logs folder)

Credits (the tools in the Tools folder are used unmodified)
------------------------------------------------------------------------------
  SWAdmin tools ............ rockon
  SW Res structs dumper .... prozaks
  7-Zip (7z.exe, 7z.dll) ... Igor Pavlov - 7-zip.org
  Xor (xor.exe) ............ Luigi Auriemma - aluigi.org
  Libraries bundled in them  Newtonsoft.Json (James Newton-King), Zydis (zyantific)
  Updater .................. ronata
==============================================================================
