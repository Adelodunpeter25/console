; NSIS installer for the Windows build of Console.
;
; scripts/build.ps1 compiles this after cargo finishes and passes the input
; paths and version through /D switches, then zips the resulting installer.
; To build it by hand from apps/desktop/scripts:
;   makensis /DAPP_EXE=C:\path\to\console.exe /DAPP_ICON=..\assets\Console.ico installer.nsi

Unicode true
ManifestDPIAware true

!include "MUI2.nsh"

!ifndef APP_EXE
  !error "APP_EXE is not defined; pass /DAPP_EXE=<path to console.exe>."
!endif
!ifndef APP_ICON
  !error "APP_ICON is not defined; pass /DAPP_ICON=<path to Console.ico>."
!endif
!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef OUTFILE
  !define OUTFILE "Console-Setup-x64.exe"
!endif

!define APP_NAME "Console"
!define APP_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Console"
!define APP_PATHS "Software\Microsoft\Windows\CurrentVersion\App Paths\console.exe"

Name "${APP_NAME}"
OutFile "${OUTFILE}"
; Per-user install: no UAC prompt, no admin rights needed.
InstallDir "$LOCALAPPDATA\Programs\Console"
InstallDirRegKey HKCU "${APP_KEY}" "InstallLocation"
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define MUI_ABORTWARNING
!define MUI_ICON "${APP_ICON}"
!define MUI_UNICON "${APP_ICON}"
!define MUI_FINISHPAGE_RUN "$INSTDIR\console.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${APP_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

; The app is a single exe (icons and UI assets are embedded), so installing is
; copying it in, wiring shortcuts and registering the uninstaller.
Section "${APP_NAME}" SecMain
  SectionIn RO
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File "${APP_EXE}"

  CreateShortCut "$SMPROGRAMS\${APP_NAME}.lnk" "$INSTDIR\console.exe" "" "$INSTDIR\console.exe" 0

  WriteRegStr HKCU "${APP_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${APP_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${APP_KEY}" "DisplayIcon" "$INSTDIR\console.exe"
  WriteRegStr HKCU "${APP_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${APP_KEY}" "Publisher" "${APP_NAME}"
  WriteRegStr HKCU "${APP_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${APP_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${APP_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${APP_KEY}" "NoRepair" 1

  ; Lets `Win+R` -> `console` launch the app.
  WriteRegStr HKCU "${APP_PATHS}" "" "$INSTDIR\console.exe"
  WriteRegStr HKCU "${APP_PATHS}" "Path" "$INSTDIR"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Desktop shortcut" SecDesktop
  SetShellVarContext current
  CreateShortCut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\console.exe" "" "$INSTDIR\console.exe" 0
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "Installs ${APP_NAME} and adds it to the Start menu."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Adds a ${APP_NAME} shortcut to the desktop."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  SetShellVarContext current
  Delete "$SMPROGRAMS\${APP_NAME}.lnk"
  Delete "$DESKTOP\${APP_NAME}.lnk"
  DeleteRegKey HKCU "${APP_KEY}"
  DeleteRegKey HKCU "${APP_PATHS}"
  ; User data lives in %LOCALAPPDATA%\Console and is intentionally left alone.
  RMDir /r "$INSTDIR"
SectionEnd
