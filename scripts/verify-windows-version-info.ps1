param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][ValidateSet('NativeAOT', 'Go')][string]$Edition,
    [Parameter(Mandatory = $true)][ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version
)

$ErrorActionPreference = 'Stop'
$info = [System.Diagnostics.FileVersionInfo]::GetVersionInfo((Resolve-Path -LiteralPath $Path).Path)
$description = if ($Edition -eq 'Go') { 'M365 MCP Server (Windows Local Go)' } else { 'M365 MCP Server (Windows Local)' }
$expected = [ordered]@{
    FileDescription = $description
    FileVersion = "$Version.0"
    ProductName = 'M365 MCP Server'
    ProductVersion = $Version
    CompanyName = 'sagehou'
    LegalCopyright = 'Copyright (c) 2026 sagehou'
}
if ($Edition -eq 'Go') {
    $expected['OriginalFilename'] = 'm365-mcp-windows-local-go.exe'
    $expected['InternalName'] = 'm365-mcp-windows-local-go'
}
foreach ($entry in $expected.GetEnumerator()) {
    if ($info.($entry.Key) -cne $entry.Value) {
        throw "$Edition $($entry.Key): expected '$($entry.Value)', got '$($info.($entry.Key))'"
    }
}
foreach ($field in @('OriginalFilename', 'InternalName', 'Comments', 'Language')) {
    if ([string]::IsNullOrWhiteSpace($info.$field)) {
        throw "$Edition version resource is missing $field"
    }
}
$info | Select-Object FileDescription, FileVersion, ProductName, ProductVersion, CompanyName, LegalCopyright, OriginalFilename, InternalName, Comments, Language | ConvertTo-Json
