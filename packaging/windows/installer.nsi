; DNSer installer for Windows (NSIS)
; Usage: makensis -DVERSION=<x.y.z> -DEXE_PATH=<path to dnser-desktop.exe> packaging/windows/installer.nsi

!define APP_NAME "DNSer"
!define COMPANY "SDK Enterprises"
!define APP_ID "enterprises.sdk.dnser.desktop"
!define REG_UNINST "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}"

Name "${APP_NAME} ${VERSION}"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\${APP_NAME}"
RequestExecutionLevel user
SetCompressor /SOLID lzma
Unicode true

VIProductVersion "${WIN_VERSION}"
VIAddVersionKey ProductName "${APP_NAME}"
VIAddVersionKey CompanyName "${COMPANY}"
VIAddVersionKey FileVersion "${VERSION}.0"
VIAddVersionKey ProductVersion "${VERSION}.0"
VIAddVersionKey FileDescription "${APP_NAME} desktop"
VIAddVersionKey LegalCopyright "Copyright (c) SDK Enterprises"

!include "MUI2.nsh"
!define MUI_ICON "${ICON_PATH}"
!define MUI_UNICON "${ICON_PATH}"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"
  File "/oname=dnser-desktop.exe" "${EXE_PATH}"

  WriteRegStr HKCU "Software\${APP_ID}" "" "$INSTDIR"

  CreateDirectory "$SMPROGRAMS\${COMPANY}"
  CreateShortcut "$SMPROGRAMS\${COMPANY}\${APP_NAME}.lnk" "$INSTDIR\dnser-desktop.exe"
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\dnser-desktop.exe"

  WriteRegStr HKCU "${REG_UNINST}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${REG_UNINST}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${REG_UNINST}" "Publisher" "${COMPANY}"
  WriteRegStr HKCU "${REG_UNINST}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${REG_UNINST}" "DisplayIcon" "$INSTDIR\dnser-desktop.exe"
  WriteRegStr HKCU "${REG_UNINST}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegStr HKCU "${REG_UNINST}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Function .onInstSuccess
  ; WebView2 runtime ships in-place; first launch falls back to 8080/8443 messaging if ports busy.
FunctionEnd

Section "Uninstall"
  Delete "$INSTDIR\dnser-desktop.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${COMPANY}\${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${COMPANY}"
  Delete "$DESKTOP\${APP_NAME}.lnk"
  DeleteRegKey HKCU "${REG_UNINST}"
  DeleteRegKey HKCU "Software\${APP_ID}"
SectionEnd
