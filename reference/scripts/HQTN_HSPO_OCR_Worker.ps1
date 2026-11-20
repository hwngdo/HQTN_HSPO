param(
  [Parameter(Mandatory=$true)][string]$PdfPath,
  [Parameter(Mandatory=$true)][string]$OutPath,
  [string]$ProgressPath = ''
)
$ErrorActionPreference = 'Stop'

# Resolve the worker folder once at script scope.
# $MyInvocation.MyCommand.Path inside a function can be $null in Windows PowerShell 5.1.
$script:WorkerRoot = $PSScriptRoot
if (-not $script:WorkerRoot -and $PSCommandPath) {
  $script:WorkerRoot = Split-Path -Parent $PSCommandPath
}
if (-not $script:WorkerRoot) {
  throw 'Cannot resolve HQTN_HSPO OCR worker folder.'
}

function Save-Result([object]$Obj) {
  $json = $Obj | ConvertTo-Json -Depth 10 -Compress
  [System.IO.File]::WriteAllText($OutPath, $json, [System.Text.UTF8Encoding]::new($false))
}

function Save-Progress([int]$Percent,[string]$Stage,[int]$Page=0,[int]$PageCount=0,[int]$Found=0) {
  if (-not $ProgressPath) { return }
  try {
    if ($Percent -lt 0) { $Percent = 0 }
    if ($Percent -gt 100) { $Percent = 100 }
    $obj = [pscustomobject]@{percent=$Percent;stage=$Stage;page=$Page;pageCount=$PageCount;found=$Found;updatedAt=[DateTime]::UtcNow.ToString('o')}
    $json = $obj | ConvertTo-Json -Compress
    $tmp = $ProgressPath + '.tmp'
    [System.IO.File]::WriteAllText($tmp,$json,[System.Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $tmp -Destination $ProgressPath -Force
  } catch { }
}

function Find-Tesseract {
  $p = Join-Path $script:WorkerRoot 'tools\tesseract\tesseract.exe'
  if ($p -and (Test-Path -LiteralPath $p)) { return (Resolve-Path -LiteralPath $p).Path }
  return $null
}

function New-ImageHelper {
$code = @"
using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
public static class HqtnCasImage {
  public static void MakeTile(string input, string output, int x, int y, int w, int h, double scale, int threshold) {
    using (var src = new Bitmap(input)) {
      x=Math.Max(0,Math.Min(x,src.Width-1)); y=Math.Max(0,Math.Min(y,src.Height-1));
      w=Math.Max(1,Math.Min(w,src.Width-x)); h=Math.Max(1,Math.Min(h,src.Height-y));
      int ow=Math.Max(1,(int)Math.Round(w*scale)); int oh=Math.Max(1,(int)Math.Round(h*scale));
      using (var tmp = new Bitmap(ow,oh,PixelFormat.Format24bppRgb)) {
        tmp.SetResolution(300,300);
        using (var g=Graphics.FromImage(tmp)) {
          g.Clear(Color.White);
          g.InterpolationMode=System.Drawing.Drawing2D.InterpolationMode.HighQualityBicubic;
          g.PixelOffsetMode=System.Drawing.Drawing2D.PixelOffsetMode.HighQuality;
          g.DrawImage(src,new Rectangle(0,0,ow,oh),new Rectangle(x,y,w,h),GraphicsUnit.Pixel);
        }
        var rect=new Rectangle(0,0,tmp.Width,tmp.Height);
        var data=tmp.LockBits(rect,ImageLockMode.ReadWrite,PixelFormat.Format24bppRgb);
        int bytes=Math.Abs(data.Stride)*data.Height; byte[] buf=new byte[bytes]; Marshal.Copy(data.Scan0,buf,0,bytes);
        for(int yy=0;yy<data.Height;yy++) {
          int row=yy*Math.Abs(data.Stride);
          for(int xx=0;xx<data.Width;xx++) {
            int i=row+xx*3; int gray=(buf[i]*29 + buf[i+1]*150 + buf[i+2]*77)>>8;
            byte v=(byte)(gray<threshold?0:255); buf[i]=v; buf[i+1]=v; buf[i+2]=v;
          }
        }
        Marshal.Copy(buf,0,data.Scan0,bytes); tmp.UnlockBits(data); tmp.Save(output,ImageFormat.Png);
      }
    }
  }
}
"@
  Add-Type -TypeDefinition $code -ReferencedAssemblies 'System.Drawing' -Language CSharp
}

try {
  Save-Progress 1 'Starting OCR...' 0 0 0
  $tess = Find-Tesseract
  if (-not $tess) {
    throw 'Embedded Tesseract OCR not found in tools\\tesseract. Run HQTN_HSPO_Tesseract_Bootstrap.ps1.'
  }

  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  Add-Type -AssemblyName System.Drawing
  $null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
  $null = [Windows.Data.Pdf.PdfDocument, Windows.Data.Pdf, ContentType = WindowsRuntime]
  $null = [Windows.Data.Pdf.PdfPageRenderOptions, Windows.Data.Pdf, ContentType = WindowsRuntime]
  $null = [Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
  $null = [Windows.Storage.Streams.DataReader, Windows.Storage.Streams, ContentType = WindowsRuntime]

  $allMethods = [System.WindowsRuntimeSystemExtensions].GetMethods()
  $script:AsTaskGeneric = $allMethods | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
  } | Select-Object -First 1
  $script:AsTaskAction = $allMethods | Where-Object {
    $_.Name -eq 'AsTask' -and -not $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction'
  } | Select-Object -First 1
  if (-not $script:AsTaskGeneric -or -not $script:AsTaskAction) { throw 'WinRT PDF async bridge could not be loaded.' }

  function Await-Operation($Op, [Type]$ResultType) {
    $m = $script:AsTaskGeneric.MakeGenericMethod($ResultType)
    $task = $m.Invoke($null, @($Op))
    return $task.GetAwaiter().GetResult()
  }
  function Await-Action($Op) {
    $task = $script:AsTaskAction.Invoke($null, @($Op))
    $task.GetAwaiter().GetResult() | Out-Null
  }

  function Render-PagePng($Page, [string]$TargetPath, [uint32]$TargetWidth=1800, [uint32]$MaxHeight=2800) {
    $mem = New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
    try {
      $opt = New-Object Windows.Data.Pdf.PdfPageRenderOptions
      $pw=[double]$Page.Size.Width; $ph=[double]$Page.Size.Height
      $dw=[uint32]$TargetWidth
      $dh=[uint32][Math]::Max(1,[Math]::Round($dw*$ph/$pw))
      if ($dh -gt $MaxHeight) { $dh=[uint32]$MaxHeight; $dw=[uint32][Math]::Max(1,[Math]::Round($dh*$pw/$ph)) }
      $opt.DestinationWidth=$dw; $opt.DestinationHeight=$dh
      Await-Action ($Page.RenderToStreamAsync($mem,$opt))
      $mem.Seek(0)
      $size=[uint32]$mem.Size
      $reader=New-Object Windows.Storage.Streams.DataReader($mem.GetInputStreamAt(0))
      try {
        $null=Await-Operation ($reader.LoadAsync($size)) ([uint32])
        $bytes=New-Object byte[] $size
        $reader.ReadBytes($bytes)
        [System.IO.File]::WriteAllBytes($TargetPath,$bytes)
      } finally { try { $reader.Dispose() } catch {} }
    } finally { try { $mem.Dispose() } catch {} }
  }

  $root = $script:WorkerRoot
  $tessData = Join-Path $root 'tools\tessdata'
  $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('HQTN_HSPO_TESS_' + [Guid]::NewGuid().ToString('N'))
  [System.IO.Directory]::CreateDirectory($tempRoot) | Out-Null
  $fullPath = [System.IO.Path]::GetFullPath($PdfPath)
  $storage = Await-Operation ([Windows.Storage.StorageFile]::GetFileFromPathAsync($fullPath)) ([Windows.Storage.StorageFile])
  $pdf = Await-Operation ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($storage)) ([Windows.Data.Pdf.PdfDocument])
  $pageCount = [int]$pdf.PageCount
  Save-Progress 7 'PDF loaded; preparing OCR...' 0 $pageCount 0
  function Test-CasChecksum([string]$Cas) {
    if ($Cas -notmatch '^([0-9]{2,7})-([0-9]{2})-([0-9])$') { return $false }
    $digits = ($Matches[1] + $Matches[2]).ToCharArray()
    $sum = 0; $weight = $digits.Length
    foreach ($ch in $digits) { $sum += ([int]::Parse([string]$ch) * $weight); $weight-- }
    return (($sum % 10) -eq [int]$Matches[3])
  }

  # Known CAS is used only as a tie-breaker for a very specific OCR table-glue case.
  # Unknown-but-valid CAS values are still accepted; the catalog is NOT used as a whitelist.
  $script:KnownCas = New-Object 'System.Collections.Generic.HashSet[string]'
  $script:CasRepairs = New-Object 'System.Collections.Generic.HashSet[string]'
  try {
    $chemPolicyPath = Join-Path $root 'policy_data\chemical_policy.json'
    if (Test-Path -LiteralPath $chemPolicyPath) {
      $chemRows = Get-Content -LiteralPath $chemPolicyPath -Raw -Encoding UTF8 | ConvertFrom-Json
      foreach ($row in @($chemRows)) {
        $cv = [string]$row.cas
        if ($cv -and (Test-CasChecksum $cv)) { $null = $script:KnownCas.Add($cv) }
      }
    }
  } catch {}

  function Resolve-CasColumnGlue([string]$Cas,[string]$Prefix) {
    if (-not (Test-CasChecksum $Cas)) { return $Cas }
    if ($script:KnownCas.Count -le 0 -or $script:KnownCas.Contains($Cas)) { return $Cas }
    $cm = [regex]::Match($Cas,'^([0-9]{3,7})-([0-9]{2})-([0-9])$')
    if (-not $cm.Success) { return $Cas }
    $first = $cm.Groups[1].Value
    if ($first.Length -lt 3) { return $Cas }
    $trimmed = $first.Substring(1) + '-' + $cm.Groups[2].Value + '-' + $cm.Groups[3].Value
    if (-not (Test-CasChecksum $trimmed) -or -not $script:KnownCas.Contains($trimmed)) { return $Cas }

    # Strong evidence 1: OCR collapsed a range endpoint into CAS, e.g. "1-5108-01-0".
    $p = [string]$Prefix
    $openRange = [regex]::Match($p,'([0-9]{1,3}(?:[.,][0-9]+)?)\s*-\s*$')
    $closedRange = [regex]::Match($p,'([0-9]{1,3}(?:[.,][0-9]+)?)\s*-\s*([0-9]{1,3}(?:[.,][0-9]+)?)\s*$')
    $repair = $openRange.Success
    if (-not $repair -and $closedRange.Success) {
      $end = $closedRange.Groups[2].Value -replace '[^0-9]',''
      if ($end.Length -gt 0 -and $first.StartsWith($end.Substring($end.Length-1,1))) { $repair = $true }
    }
    if ($repair) {
      $null = $script:CasRepairs.Add($Cas + '=>' + $trimmed)
      return $trimmed
    }
    return $Cas
  }

  function Get-ValidCas([string]$Text) {
    $set = New-Object 'System.Collections.Generic.HashSet[string]'
    $src = [string]$Text
    # Precision rule: exactly two hyphens; never bridge whitespace between independent number columns.
    foreach ($m in [regex]::Matches($src,'(?<![0-9])([0-9]{2,7})\s*-\s*([0-9]{2})\s*-\s*([0-9])(?![0-9])')) {
      $c = $m.Groups[1].Value + '-' + $m.Groups[2].Value + '-' + $m.Groups[3].Value
      if (-not (Test-CasChecksum $c)) { continue }
      $start = [Math]::Max(0,$m.Index-28)
      $prefix = $src.Substring($start,$m.Index-$start)
      $resolved = Resolve-CasColumnGlue $c $prefix
      if (Test-CasChecksum $resolved) { $null = $set.Add($resolved) }
    }
    return @($set)
  }

  function Invoke-TessRaw([string]$Png,[int]$PageNo,[int]$PassNo,[int]$Psm) {
    $outBase = Join-Path $tempRoot (('raw_{0:000}_{1:00}' -f $PageNo,$PassNo))
    $args = @($Png,$outBase,'--psm',[string]$Psm,'-l','eng')
    if (Test-Path -LiteralPath (Join-Path $tessData 'eng.traineddata')) { $args = @('--tessdata-dir',$tessData) + $args }
    $errPath = $outBase + '.err.txt'
    $p = Start-Process -FilePath $tess -ArgumentList $args -NoNewWindow -PassThru -Wait -RedirectStandardError $errPath
    if ($p.ExitCode -eq 0 -and (Test-Path -LiteralPath ($outBase + '.txt'))) {
      return [System.IO.File]::ReadAllText($outBase + '.txt')
    }
    return ''
  }

  function Get-Consensus([string]$TextA,[string]$TextB) {
    $a = New-Object 'System.Collections.Generic.HashSet[string]'
    $b = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($c in (Get-ValidCas $TextA)) { $null = $a.Add($c) }
    foreach ($c in (Get-ValidCas $TextB)) { $null = $b.Add($c) }
    $out = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($c in $a) { if ($b.Contains($c)) { $null = $out.Add($c) } }
    return @($out)
  }

  function Set-PageConfirmed($State,[string[]]$Values,[string]$Mode) {
    $State.confirmed.Clear()
    foreach ($c in @($Values)) { if ($c) { $null = $State.confirmed.Add([string]$c) } }
    $State.mode = $Mode
  }

  $pageStates = New-Object System.Collections.Generic.List[object]
  $pages = New-Object System.Collections.Generic.List[object]
  $highResPages = New-Object System.Collections.Generic.List[int]
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    # PASS 1: fast raw full-page OCR at moderate resolution. No thresholding/cropping.
    # Run PSM 3 on every page. Only pages that contain a checksum-valid CAS candidate
    # receive the second PSM 4 confirmation pass. This keeps precision while avoiding
    # an unnecessary second Tesseract process on pages that clearly contain no CAS.
    for ($i=0; $i -lt [int]$pdf.PageCount; $i++) {
      $pageNo = $i + 1
      $p0 = 10 + [int][Math]::Floor((60.0 * $i) / [Math]::Max(1,$pageCount))
      Save-Progress $p0 ('Fast OCR page ' + $pageNo + '/' + $pageCount + ' - rendering') $pageNo $pageCount 0
      $page = $pdf.GetPage([uint32]$i)
      try {
        $png = Join-Path $tempRoot (('page_{0:000}_raw.png' -f $pageNo))
        Render-PagePng $page $png 1800 2800
        $txt3 = Invoke-TessRaw $png $pageNo 1 3
        $cand3 = @(Get-ValidCas $txt3)
        $p1 = 10 + [int][Math]::Floor((60.0 * ($i + 0.70)) / [Math]::Max(1,$pageCount))
        Save-Progress $p1 ('Fast OCR page ' + $pageNo + '/' + $pageCount + ' - screening') $pageNo $pageCount $cand3.Count
        $cons = @()
        $passes = 1
        $mode = 'raw-screen'
        if ($cand3.Count -gt 0) {
          $txt11 = Invoke-TessRaw $png $pageNo 2 4
          $cons = @(Get-Consensus $txt3 $txt11)
          $passes = 2
          $mode = 'raw-consensus'
        }
        $p2 = 10 + [int][Math]::Floor((60.0 * ($i + 1.0)) / [Math]::Max(1,$pageCount))
        Save-Progress $p2 ('Fast OCR page ' + $pageNo + '/' + $pageCount + ($(if($passes -eq 2){' - confirmed'}else{' - no CAS candidate'}))) $pageNo $pageCount $cons.Count
        $fast = New-Object 'System.Collections.Generic.HashSet[string]'
        $confirmed = New-Object 'System.Collections.Generic.HashSet[string]'
        foreach ($c in $cand3) { $null = $fast.Add($c) }
        foreach ($c in $cons) { $null = $confirmed.Add($c) }
        $pageStates.Add([pscustomobject]@{
          page=$pageNo; fast=$fast; confirmed=$confirmed; hq=$null; mode=$mode; passes=$passes
        })
      } finally { try { $page.Dispose() } catch {} }
    }

    # If no page has a composition-like cluster (2+ CAS), verify at higher resolution.
    # This also re-checks isolated candidates so accidental date/table numbers do not win.
    $hasCluster = $false
    foreach ($st in $pageStates) { if ($st.confirmed.Count -ge 2) { $hasCluster = $true; break } }
    if (-not $hasCluster) {
      $priority = New-Object System.Collections.Generic.List[int]
      foreach ($n in @(2,3,4,1,5,6,7,8)) {
        if ($n -le [int]$pdf.PageCount -and -not $priority.Contains($n)) { $priority.Add($n) }
      }
      $hqIndex = 0
      foreach ($pageNo in $priority) {
        $hqIndex++
        $hqP0 = 72 + [int][Math]::Floor((22.0 * ($hqIndex - 1)) / [Math]::Max(1,$priority.Count))
        Save-Progress $hqP0 ('Quality OCR page ' + $pageNo + '/' + $pageCount + ' - rendering') $pageNo $pageCount 0
        $page = $pdf.GetPage([uint32]($pageNo-1))
        try {
          $pngHi = Join-Path $tempRoot (('page_{0:000}_hqraw.png' -f $pageNo))
          Render-PagePng $page $pngHi 2600 4000
          $txt3 = Invoke-TessRaw $pngHi $pageNo 11 3
          $hqMid = 72 + [int][Math]::Floor((22.0 * ($hqIndex - 0.5)) / [Math]::Max(1,$priority.Count))
          Save-Progress $hqMid ('Quality OCR page ' + $pageNo + '/' + $pageCount + ' - pass 1/2') $pageNo $pageCount 0
          $txt11 = Invoke-TessRaw $pngHi $pageNo 12 4
          $hqCons = @(Get-Consensus $txt3 $txt11)
          $hqDone = 72 + [int][Math]::Floor((22.0 * $hqIndex) / [Math]::Max(1,$priority.Count))
          Save-Progress $hqDone ('Quality OCR page ' + $pageNo + '/' + $pageCount + ' - pass 2/2') $pageNo $pageCount $hqCons.Count
          $hq = New-Object 'System.Collections.Generic.HashSet[string]'
          foreach ($c in $hqCons) { $null = $hq.Add($c) }
          $st = $pageStates[$pageNo-1]
          $st.hq = $hq
          Set-PageConfirmed $st $hqCons 'hq-raw-consensus'
          $st.passes = 4
          $null = $highResPages.Add($pageNo)
          if ($st.confirmed.Count -ge 2) { $hasCluster = $true; break }
        } finally { try { $page.Dispose() } catch {} }
      }
    }

    # Precision policy:
    # - If any page contains 2+ independently confirmed CAS, treat those pages as the
    #   composition table and ignore isolated CAS-shaped numbers on other pages.
    # - Otherwise keep only independently confirmed single CAS results.
    Save-Progress 95 'Validating CAS candidates...' 0 $pageCount 0
    $clusterExists = $false
    foreach ($st in $pageStates) { if ($st.confirmed.Count -ge 2) { $clusterExists = $true; break } }
    $selected = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($st in $pageStates) {
      $keep = (-not $clusterExists) -or ($st.confirmed.Count -ge 2)
      $outCas = New-Object System.Collections.Generic.List[string]
      if ($keep) {
        foreach ($c in $st.confirmed) { $outCas.Add($c); $null = $selected.Add($c) }
      }
      # Return only curated CAS lines to the browser. Raw OCR noise must not be re-parsed there.
      $pages.Add([pscustomobject]@{
        page=$st.page; text=[string]::Join("`n",$outCas.ToArray()); passes=$st.passes; mode=$st.mode; confirmed=$outCas.ToArray()
      })
    }
  } finally {
    $sw.Stop()
    try { $pdf.Dispose() } catch {}
    try { Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  }

  $ver = 'unknown'
  try { $ver = (& $tess --version 2>$null | Select-Object -First 1) } catch {}
  Save-Progress 99 'Preparing OCR result...' 0 $pageCount $selected.Count
  Save-Result ([pscustomobject]@{
    ok=$true; engine='Tesseract CAS offline'; tesseract=$ver; pageCount=$pages.Count; pages=$pages.ToArray();
    diagnostics=[pscustomobject]@{selectedCas=@($selected);highResPages=$highResPages.ToArray();elapsedMs=$sw.ElapsedMilliseconds;strategy='fast-screen-psm3-confirm-psm4-v3-cas-column-guard';repairs=@($script:CasRepairs)}
  })
  Save-Progress 100 'Completed' 0 $pageCount $selected.Count
  exit 0
} catch {
  Save-Progress 100 'OCR failed' 0 0 0
  Save-Result ([pscustomobject]@{ok=$false;error=$_.Exception.Message;engine='Tesseract CAS offline'})
  exit 3
}
