SW TL Updater 2.20.1  (by ronata)
==============================================================================
Patches SoulWorker's data12.v with your translated .csv files.
Uses the same tools as before (the Tools folder) - nothing there is replaced.

IMPORTANT - please read
  - Unofficial fan project, not affiliated with or approved by the game's publishers. It has no permission from them.
  - The game's terms restrict modifying game data. Using this tool could get your account suspended or banned.
    Use it at your own risk, and not on an account you cannot afford to lose.
  - No game files are included or uploaded: the patched file is built on your own PC from your own copy of the game.

Folder layout (everything next to SWTLUpdater.exe)
------------------------------------------------------------------------------
  SWTLUpdater.exe
  Tools\            7z and the SW Res structs dumper (see "Credits"). They come in the player zip;
                    if yours has no Tools folder, the program tells you which files it needs and where to get them
  TL csv\           Your translated .csv files

Created automatically:
  SWTLUpdater.ini   Your settings
  backup\           Previous patched files
  logs\             update_<date>.log = one per table patch; online_update.log = every update check (versions, files, patch vs whole file, sizes, errors)
  cache\            Saved table schema and update notes (speeds things up)
  data12.v          The newest patched file

How to use
------------------------------------------------------------------------------
 1. Close SoulWorker, and close SWPatcher or make sure it is idle (not waiting for the client).
    (The updater warns you if either one is running, and only one updater can run at a time.)
 2. Double-click SWTLUpdater.exe.
 3. First time: the starting settings are shown. Press Enter to accept them,
    or type C to change them (a folder window opens when you press Enter at a folder question).
 4. Choose 1) Patch now. Wait for "Patching complete".
    Not sure about something? 2) Dry run does everything except save files.

Menu
------------------------------------------------------------------------------
  1) Patch now        
  2) Dry run        
  3) Settings
  4) Restore a backup (puts a previous patched file back into the patcher folder)
  5) Open logs folder                   
  6) About, credits & help
  7) Check for updates                  
  0) Exit

Settings and their default values
------------------------------------------------------------------------------
Change them in the menu (3) Settings or by editing SWTLUpdater.ini with Notepad.

  Setting (in the menu)       Key in the .ini      Default value
  --------------------------  -------------------  ---------------------------------------------
  Game folder                 GameFolder           C:\Wemade Online\Soulworker
  Translation folder          TranslationFolder    <program folder>\TL csv
  Patcher folder              PatcherFolder        %LocalAppData%\SWPatcher\jp\English\datas
  Game data file to patch     SourceDataFile       data12            (read from <game>\datas\data12.v)
  Name of the patched file    OutputDataFile       data12
  Backups to keep             BackupsToKeep        5                 (0 = keep all)
  Tables to skip              SkipTables           tb_Loading_String,tb_Option_String,tb_Talk_String
  Ask settings at start       AskAtStartup         yes               (yes / no)
  Update address              UpdateURL            (empty = online updates off; comes with your download)
  Update key                  UpdateKey            (empty; comes with your download - don't change it)
  (Big translation tables update with small patches when possible: only the changed rows are downloaded.)
  Translation address         TranslationURL       (optional; only in the .ini. Where the translation files come from,
                                                    when they are kept in a separate place. Comes with your download)
  Translation key             TranslationKey       (optional; empty = same key as UpdateKey)
  Check for updates at start  UpdateAtStartup      yes               (yes / no)
  Table maker (advanced)      Converter            builtin           (builtin = built into the updater, the default;
                                                    swadmin = use the older SWAdmin tool instead, needs Tools\SWAdmin;
                                                    check = do both, compare, report, and use SWAdmin's result)

  - "Reset everything to the default values" in the Settings menu restores the table above
    (your update address, translation address and keys are kept).
  - Update address: - (a dash) turns all online updates off, including the translation.
  - the program asks for the folders the first time and remembers them in SWTLUpdater.ini.

Optional command-line switches (for shortcuts or scripts; no menu is shown)
------------------------------------------------------------------------------
  --run          patch now              
  --dry-run      check everything, save nothing
  --restore      restore newest backup  
  --strict       CSV problems stop the run
  --keep-work    keep the temp folder   
  --settings     open the settings menu
  --update       check for updates and install them (add --run to patch afterwards)
  --appdir PATH  use another folder for Tools / TL csv / settings
  Exit codes: 0 ok, 1 error, 3 nothing changed.

What it checks for you
------------------------------------------------------------------------------
  - Every tool step; a failure stops the run with a clear message.
  - Your CSVs: valid UTF-8 and the same number of columns on every row.
  - The rebuilt data12 must contain exactly the original files (none lost, none duplicated),
    tables are stored like the originals (ZipCrypto + Deflate), and the whole archive is
    extracted and compared before anything is saved.
  - The previous patcher file is backed up before it is replaced; results are saved safely
    (temp file + rename), and every run writes a log.

After a game update
------------------------------------------------------------------------------
  When the game is updated, tables can change before the translation does. For every table the updater
  compares the translation with the game's own table first:
   - Rows the game added that the translation does not have yet are kept in the game's own language
     (the log lists how many per table), so the game keeps working and everything already translated stays.
   - Rows are matched by the ID in the first column, never by position. Before a table is written the
     updater checks that every ID of the game's table is still there, in the game's order, and that
     every number is the game's. If that check fails, the table is left as the game has it.
   - The rows the game has and the translation does not are also saved as CSV in logs\new_rows\<table>.csv
     (same format as the translation files), so they can be translated and added.
   - Self-test: for every table the updater first writes the game's own rows back and checks that the
     result is byte for byte the game's table. If not, that table is left as the game has it.
   - Rows the game removed are left out of the table.
   - Number columns always come from the game itself (only the text comes from the translation), so a game
     update that changes values is never overwritten by older numbers.
   - If the number columns of the translation do not match the game's (columns were moved), that table is
     left as the game has it. If text columns that are codes or empty in the game now hold other text, the
     table is still patched but a warning asks you to report it.
   - If a table's layout changed (a column was added or removed) or its rows were renumbered, only THAT
     table is left exactly as the game has it. All other tables are still translated.
   - If more than half of the tables cannot be used, nothing is saved at all.
  Nothing else is needed from you: run the updater again after a new translation is published.
  If you have any suggestion or problem, ask ronata on Discord and send the newest file from the logs folder.

Game updates and repairs
------------------------------------------------------------------------------
  After every patch the updater remembers which game files it saw (a fingerprint, nothing is uploaded).
  At the next start, if the game's data file or SoulWorker64.dll is different, it tells you the game was
  updated (or the file was replaced) and that you should choose 1) Patch now again. 4) Restore a backup
  is there if something looks wrong afterwards. No message means nothing changed.

For the translation team: export the reference tables
------------------------------------------------------------------------------
  SWTLUpdater.exe -export-ref -region jp|kr|gb -game "<game folder>" [-pwfile "<password.txt>"] [-out "<folder>"]
  does what the old dumper + Get-res batch files did, in one step: game version (Ver.ini), table layout
  (schema.json), decrypt and extract data12.v (data11.v for gb), and keep only the string/script tables the
  translation uses. Result: <out>\<REGION>\<version>\res\*.res (Global: ENG-res, TWN-res, CHN-res) plus
  schema.json, Ver.ini, build.txt and files.sha256. The game folder is never changed. JP has a built-in
  archive key (see below), used when -pwfile is not given. -pwfile may be a plain one-line file or a list with
  lines like "data12 = ..." (a line starting with a dot is an old password and is ignored); the line for the data
  file of that region is used (data12, or data11 for gb). 5_export_reference.bat (publisher package) runs all regions.

Archive key
------------------------------------------------------------------------------
  The program needs the game's data password. It is stored as encrypted text (a line starting with SWTLKEY1:
  at the end of this file), so you never see or type it. Keep this file as it came. If the game changes its
  password you get a new line with the next update, or you are sent one to paste into SWTLUpdater.ini as
  ArchiveKey=<the line>. (Publisher: 6_make_archive_key.bat makes the line.)

Troubleshooting
------------------------------------------------------------------------------
  - "Tools folder is incomplete": copy the Tools folder next to the exe.
  - "SoulWorker not found": Settings > Game folder.
  - Problems? Send the newest file from the logs folder (menu 5) to Discord: ronata
  - Needs 64-bit Windows 10 or newer. Windows may warn about an "unknown publisher"
    because the exe is not code-signed: More info > Run anyway.

Online updates
------------------------------------------------------------------------------
  If your download came with an update address and key, the program checks for new versions when it starts
  (or press 7). It asks before installing, and verifies a digital signature on everything.
  There can be two places to check:
    - the program source: new versions of SWTLUpdater.exe, this readme and the Tools
    - the translation source (if TranslationURL is set): new or changed translation files in TL csv\
  The translation is checked first, then the program; if the program updated itself it restarts by itself.
  Updates can only touch SWTLUpdater.exe, README.txt, Tools\ and TL csv\ - the translation source can only
  touch TL csv\. If one source can't be reached, the other one is still checked.
  Updating does not patch the game by itself: choose 1) Patch now afterwards (or use --update --run).
  Turn it off in Settings (Update address: -).
  If an update fails with "signature check FAILED", nothing was changed - do not change the key, ask on Discord.

Help / contact
------------------------------------------------------------------------------
  Any question or issue at Discord: ronata   (attach the newest file from the logs folder)

Credits (the tools in the Tools folder are used unmodified; the xor step is now built into the updater)
------------------------------------------------------------------------------
  SWAdmin tools ............ GreenDamTan (command-line version credited to rockon)  (the schema and table steps are built in now; first done with SWAdmin)
  SW Res structs dumper .... Asaduji - github.com/Asaduji/SW-Res-structs-dumper
  Xor step (built in now) .. first done with Xor by Luigi Auriemma - aluigi.org (xor.exe is no longer needed)
  7-Zip (7z.exe, 7z.dll) ... Igor Pavlov - 7-zip.org  (licensed under the GNU LGPL; source code at www.7-zip.org)
  Libraries bundled in them  Newtonsoft.Json (James Newton-King), Zydis (zyantific)
  Updater .................. ronata
==============================================================================

Archive key (do not edit):
SWTLKEY1:data12:pzFn9LGLwvdGiutqZPvrEvsNHUjzEjcE0AtHZY3JSgYAKWnZtLCUtrad8qDbtg==
