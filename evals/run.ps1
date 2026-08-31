param(
    [string]$Model = "anthropic/claude-sonnet-4.5",
    [string]$Output = "eval_report.md"
)

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host " ActonOS Cognition & Reliability Eval Suite " -ForegroundColor Cyan
Write-Host " Target Model: $Model" -ForegroundColor Gray
Write-Host " Output Report: $Output" -ForegroundColor Gray
Write-Host "==========================================" -ForegroundColor Cyan

go run ./evals/runner/main.go --mode=mock --model=$Model --output=$Output --fail-under=90.0
