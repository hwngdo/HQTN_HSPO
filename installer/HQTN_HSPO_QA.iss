#ifndef Stage
  #error Stage must point to the verified QA installer payload
#endif
#ifndef Out
  #error Out must point to the installer output directory
#endif
#ifndef AppRelease
  #error AppRelease must contain the application release version
#endif

[Setup]
AppId={{BDB1CA22-DF58-4474-929C-6D5938D32A9C}
AppName=HQTN_HSPO QA
AppVersion={#AppRelease}
DefaultDirName={localappdata}\Programs\HQTN_HSPO_QA
PrivilegesRequired=lowest
ArchitecturesAllowed=x64os
ArchitecturesInstallIn64BitMode=x64os
MinVersion=10.0
DisableProgramGroupPage=yes
DisableDirPage=no
CloseApplications=yes
CloseApplicationsFilter=HQTN_HSPO_Tray_QA.exe,HQTN_HSPO_Core_QA.exe
RestartApplications=no
SetupIconFile={#Stage}\static\hspo.ico
UninstallDisplayIcon={app}\static\hspo.ico
OutputDir={#Out}
OutputBaseFilename=HQTN_HSPO_QA_Setup_Win64
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[CustomMessages]
LaunchQA=Mở HQTN_HSPO QA

[Files]
Source: "{#Stage}\HQTN_HSPO_Core_QA.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\HQTN_HSPO_Tray_QA.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\HQTN_HSPO_OCR_Worker.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\VERSION.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\APP_RELEASE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\HQTN_HSPO_QA_INSTALL.txt"; DestDir: "{app}"; Flags: onlyifdoesntexist uninsneveruninstall
Source: "{#Stage}\static\*"; Excludes: "*.json,*.json.gz,\documents\*"; DestDir: "{app}\static"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#Stage}\static\*.json"; DestDir: "{app}\static"; Flags: onlyifdoesntexist uninsneveruninstall
Source: "{#Stage}\static\*.json.gz"; DestDir: "{app}\static"; Flags: onlyifdoesntexist uninsneveruninstall
Source: "{#Stage}\static\documents\*"; DestDir: "{app}\static\documents"; Flags: onlyifdoesntexist uninsneveruninstall recursesubdirs createallsubdirs
Source: "{#Stage}\policy_data\*"; DestDir: "{app}\policy_data"; Flags: onlyifdoesntexist uninsneveruninstall recursesubdirs createallsubdirs
Source: "{#Stage}\tools\tesseract\*"; DestDir: "{app}\tools\tesseract"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\HQTN_HSPO QA"; Filename: "{app}\HQTN_HSPO_Tray_QA.exe"; Parameters: "-qa -qa-port=18972"; WorkingDir: "{app}"; IconFilename: "{app}\static\hspo.ico"

[Run]
Filename: "{app}\HQTN_HSPO_Tray_QA.exe"; Parameters: "-qa -qa-port=18972"; WorkingDir: "{app}"; Description: "{cm:LaunchQA}"; Flags: postinstall nowait skipifsilent

[Code]
var
  AdminPage: TInputQueryWizardPage;
  BootstrapPassword: String;

function SetEnvironmentVariableW(Name, Value: String): LongBool;
  external 'SetEnvironmentVariableW@kernel32.dll stdcall';

function HasAccounts: Boolean;
begin
  Result := FileExists(ExpandConstant('{app}\data\users.json'));
end;

procedure InitializeWizard;
begin
  AdminPage := CreateInputQueryPage(wpSelectDir, 'Tạo tài khoản Admin QA',
    'Dùng tài khoản admin để cập nhật dữ liệu.',
    'Đặt mật khẩu ít nhất 12 ký tự cho lần cài đầu.');
  AdminPage.Add('Mật khẩu:', True);
  AdminPage.Add('Nhập lại mật khẩu:', True);
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := (PageID = AdminPage.ID) and (WizardSilent or HasAccounts);
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = AdminPage.ID then
  begin
    Result := (Length(Trim(AdminPage.Values[0])) >= 12) and
      (AdminPage.Values[0] = AdminPage.Values[1]);
    if not Result then
      MsgBox('Mật khẩu cần ít nhất 12 ký tự và hai lần nhập phải giống nhau.', mbError, MB_OK);
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if FileExists(ExpandConstant('{app}\HQTN_HSPO_Core.exe')) or
     FileExists(ExpandConstant('{app}\HQTN_HSPO.exe')) or
     DirExists(ExpandConstant('{app}\userdata')) or
     ((HasAccounts or FileExists(ExpandConstant('{app}\HQTN_HSPO_Tray_QA.exe'))) and
      not FileExists(ExpandConstant('{app}\HQTN_HSPO_QA_INSTALL.txt'))) then
  begin
    Result := 'Chọn một thư mục riêng cho bộ cài QA.';
    Exit;
  end;
  if not HasAccounts then
  begin
    if WizardSilent then
      BootstrapPassword := GetEnv('HSPO_RECOVERY_QA_ADMIN_PASSWORD')
    else
      BootstrapPassword := AdminPage.Values[0];
    if Length(Trim(BootstrapPassword)) < 12 then
      Result := 'Cần mật khẩu Admin QA ít nhất 12 ký tự để khởi tạo.';
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  OK: Boolean;
  ExitCode: Integer;
begin
  if (CurStep <> ssPostInstall) or HasAccounts then
    Exit;
  if not SetEnvironmentVariableW('HSPO_RECOVERY_QA_ADMIN_PASSWORD', BootstrapPassword) then
    RaiseException('Không truyền được thông tin khởi tạo Admin QA.');
  OK := SetEnvironmentVariableW('HSPO_ALLOW_INSECURE_RECOVERY_QA', 'I_ACKNOWLEDGE_INSECURE_QA_ONLY');
  if OK then
    OK := Exec(ExpandConstant('{app}\HQTN_HSPO_Core_QA.exe'), '-initialize-only',
      ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, ExitCode);
  SetEnvironmentVariableW('HSPO_RECOVERY_QA_ADMIN_PASSWORD', '');
  SetEnvironmentVariableW('HSPO_ALLOW_INSECURE_RECOVERY_QA', '');
  BootstrapPassword := '';
  if not OK or (ExitCode <> 0) or not HasAccounts then
    RaiseException('Không khởi tạo được Admin QA.');
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Names: TArrayOfString;
  Index: Integer;
  RunKey, Command, Prefix: String;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  RunKey := 'Software\Microsoft\Windows\CurrentVersion\Run';
  Prefix := '"' + ExpandConstant('{app}\HQTN_HSPO_Tray_QA.exe') + '" -qa -background -qa-port=';
  if RegGetValueNames(HKCU, RunKey, Names) then
    for Index := 0 to GetArrayLength(Names) - 1 do
      if (Pos('HQTN_HSPO_QA_', Names[Index]) = 1) and
         RegQueryStringValue(HKCU, RunKey, Names[Index], Command) and
         (Pos(Lowercase(Prefix), Lowercase(Command)) = 1) then
        RegDeleteValue(HKCU, RunKey, Names[Index]);
end;
