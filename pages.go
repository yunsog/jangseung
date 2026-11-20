package main

// 화면 틀. 문구는 {{call .T "키"}} 로 사전에서. html/template 이 자동으로 이스케이프함.
const templateText = `
{{define "head"}}<!doctype html><html lang="{{.Lang}}"{{if .Theme}} data-theme="{{.Theme}}"{{end}}><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>{{call .T "title"}}</title>{{if .Refresh}}<meta http-equiv="refresh" content="4">{{end}}<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="{{asset "style.css"}}"></head><body>
<header class="top"><div class="bar"><a href="/" class="logo">{{if eq .Theme "night"}}<img src="/static/jangseung_logo_dark.svg" alt="장승 Jangseung" height="44">{{else if eq .Theme "day"}}<img src="/static/jangseung_logo.svg" alt="장승 Jangseung" height="44">{{else}}<picture><source media="(prefers-color-scheme: dark)" srcset="/static/jangseung_logo_dark.svg"><img src="/static/jangseung_logo.svg" alt="장승 Jangseung" height="44"></picture>{{end}}</a><nav>
<a href="/" {{if eq .Active "status"}}class="on"{{end}}>{{call .T "nav_status"}}</a>
<a href="/explorer" {{if eq .Active "explorer"}}class="on"{{end}}>{{call .T "nav_explorer"}}</a>
<a href="/list" {{if eq .Active "list"}}class="on"{{end}}>{{call .T "nav_list"}}</a>
<a href="/history" {{if eq .Active "history"}}class="on"{{end}}>{{call .T "nav_history"}}</a>
<a href="/changes" {{if eq .Active "changes"}}class="on"{{end}}>{{call .T "nav_changes"}}</a>
<a href="/repos" {{if eq .Active "ops"}}class="on"{{end}}>{{call .T "nav_ops"}}</a>
<a href="/internal/" id="nav-int" {{if eq .Active "internal"}}class="on"{{else}}hidden{{end}}>{{call .T "nav_internal"}}</a></nav>
<div class="tools">{{if .Health}}<span class="pill {{if eq .Health.Status "ok"}}ok{{else}}bad{{end}}" title="{{call .T "serial"}} {{.Health.Serial}}">{{if eq .Health.Status "ok"}}{{call .T "ok"}}{{else}}{{.Health.Status}}{{end}} · #{{.Health.Serial}}</span>{{end}}
<span class="seg"><a class="sw {{if eq .Lang "ko"}}on{{end}}" href="{{.With "lang" "ko"}}">한국어</a><a class="sw {{if eq .Lang "en"}}on{{end}}" href="{{.With "lang" "en"}}">English</a></span>
<span class="seg"><a class="sw {{if eq .Theme "day"}}on{{end}}" href="{{.With "theme" "day"}}" title="Day">☀ Day</a><a class="sw {{if eq .Theme "night"}}on{{end}}" href="{{.With "theme" "night"}}" title="Night">☾ Night</a>{{if .Theme}}<a class="sw" href="{{.With "theme" "auto"}}" title="{{call .T "theme_auto"}}">Auto</a>{{end}}</span></div></div></header>
<main>{{if .Err}}<p class="err">{{.Err}}</p>{{end}}{{end}}

{{define "foot"}}</main><script src="{{asset "app.js"}}"></script><footer>{{call .T "source"}}{{if .Status}} · {{call .T "as_of"}} {{time .Status.DataTime}}{{end}}{{if .Health}} · {{call .T "serial"}} {{.Health.Serial}} · {{call .T "age"}} {{.Health.DataAgeS}}{{call .T "sec"}}{{end}}{{if .Stale}} · <span class="warn">{{call .T "stale"}}</span>{{end}}</footer></body></html>{{end}}

{{define "vrps"}}{{$t := .T}}<div class="wrap"><table><tr><th>{{call $t "prefix"}}</th><th>{{call $t "max_length"}}</th><th>{{call $t "asn"}}</th><th>{{call $t "ta"}}</th><th>{{call $t "expires"}}</th><th>{{call $t "roa"}}</th></tr>
{{range .Rows}}<tr{{if .NoMatch}} class="nomatch"{{end}}><td><a href="/explorer?prefix={{.Prefix}}">{{.Prefix}}</a></td><td>{{.MaxLength}}{{if .NoMatch}} <span class="tag" title="{{printf (call $t "no_match_why") .TargetLen .MaxLength}}">{{call $t "no_match"}}</span>{{end}}</td><td><a href="/explorer?q=AS{{.ASN}}">{{asn .ASN}}</a></td><td>{{.TA}}</td><td>{{epoch .Expires}}</td>
<td>{{range .ROAs}}<span class="mut" title="{{.}}">{{roaName .}}</span> {{end}}</td></tr>{{end}}</table></div>{{end}}

{{define "status"}}{{template "head" .}}{{$t := .T}}
{{if .Health}}<p><span class="b {{if eq .Health.Status "ok"}}ok{{else}}bad{{end}}">{{if eq .Health.Status "ok"}}{{call $t "ok"}}{{else}}{{call $t "bad"}} ({{.Health.Status}}){{end}}</span></p>{{end}}
{{with .Status}}<div class="card grid">
<div class="stat"><span>VRP</span><b>{{num .LastRun.VRPs}}</b></div>
<div class="stat"><span>ASPA</span><b>{{num .LastRun.VAPs}}</b></div>
<div class="stat"><span>{{call $t "router_keys"}}</span><b>{{.LastRun.RouterKeys}}</b></div>
<div class="stat"><span>{{call $t "changes"}}</span><b>+{{.LastRun.Announced}} / -{{.LastRun.Withdrawn}}</b></div>
<div class="stat"><span>{{call $t "took"}}</span><b>{{printf "%.0f" .LastRun.DurationS}}{{call $t "sec"}}</b></div>
<div class="stat"><span>{{call $t "last_run"}}</span><b>{{if .LastRun.Applied}}{{call $t "applied"}}{{else}}{{call $t "not_applied"}}{{end}}</b></div></div>
<div class="card"><b>{{call $t "per_ta"}}</b><table>{{range $.Data}}<tr><td>{{.TA}}</td><td class="n">{{num .N}}</td></tr>{{end}}</table></div>{{end}}
{{template "foot" .}}{{end}}

{{define "validbody"}}{{$t := .T}}{{with .V.Route}}<p class="verdict">{{.Route.Origin}} {{.Route.Prefix}} → <span class="b {{.Validity.State}}">{{.Validity.State}}</span>
<span class="mut">{{.Validity.Description}}</span></p>
{{template "short" (list2 $t "matched" .Validity.VRPs.Matched)}}
{{template "short" (list2 $t "unmatched_as" .Validity.VRPs.UnmAS)}}
{{template "short" (list2 $t "unmatched_length" .Validity.VRPs.UnmLength)}}{{end}}{{end}}

{{define "aspabody"}}{{$t := .T}}{{with .R}}<p class="verdict">upstream <span class="b {{.Result.Upstream.State}}">{{.Result.Upstream.State}}</span> <span class="mut">{{.Result.Upstream.Reason}}</span><br>
downstream <span class="b {{.Result.Downstream.State}}">{{.Result.Downstream.State}}</span> <span class="mut">{{.Result.Downstream.Reason}}</span></p>
{{if .Result.AsSet}}<p class="warn">AS_SET</p>{{end}}
<div class="wrap"><table><tr><th>hop</th><th>upstream</th><th>downstream</th></tr>{{range .Result.Hops}}<tr><td><a href="/explorer?q=AS{{.From}}">AS{{.From}}</a> → <a href="/explorer?q=AS{{.To}}">AS{{.To}}</a></td><td>{{.Up}}</td><td>{{.Down}}</td></tr>{{end}}</table></div>{{end}}{{end}}

{{define "explorer"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "sec_input"}}</h2></div>
<form><label class="grow">{{call $t "q_label"}}<input name="q" value="{{.Q}}" placeholder="{{call $t "q_ph"}}" size="64" autofocus></label>
<label>{{call $t "match"}}<select name="prefixMatch">
<option value="any" {{if eq .Match "any"}}selected{{end}}>{{call $t "m_any"}}</option>
<option value="exact" {{if eq .Match "exact"}}selected{{end}}>{{call $t "m_exact"}}</option>
<option value="lspec" {{if eq .Match "lspec"}}selected{{end}}>{{call $t "m_lspec"}}</option>
<option value="mspec" {{if eq .Match "mspec"}}selected{{end}}>{{call $t "m_mspec"}}</option></select></label><button>{{call $t "search"}}</button></form>
<p class="mut note">{{call $t "q_desc"}}</p></section>
{{with .V}}<section class="card"><div class="card-h"><h2>{{call $t "nav_route"}}</h2></div>{{template "validbody" (vb $t .)}}</section>{{end}}
{{with .Aspa}}<section class="card"><div class="card-h"><h2>{{call $t "nav_aspa"}}</h2></div>{{template "aspabody" (ab $t .)}}</section>{{end}}
{{with .AS}}<section class="card"><div class="card-h"><h2>{{call $t "nav_as"}} <span class="count">{{asn .ASN}}</span></h2></div>
<div class="grid"><div class="stat"><span>{{call $t "router_keys"}}</span><b>{{len .RouterKeys}}</b></div>
<div class="stat"><span>ASPA ({{call $t "aspa_prov"}})</span><b>{{if .ASPA}}{{len .ASPA.Providers}}{{else}}0{{end}}</b></div>
<div class="stat"><span>{{call $t "provider_of"}}</span><b>{{len .ProviderOf}}</b></div>
<div class="stat"><span>VRP</span><b>{{num $.Data.ASTotal}}</b></div></div></section>
<section class="card"><div class="card-h"><h2>{{call $t "router_keys"}} <span class="count">{{len .RouterKeys}}</span></h2></div>
{{if $.Data.Keys}}{{template "keystable" (kt $t $.Data.Keys)}}{{else if $.Data.KeysLoading}}<p class="mut">{{call $t "loading"}}</p>{{else}}<p class="mut">{{call $t "none"}}</p>{{end}}</section>
<section class="card"><div class="card-h"><h2>ASPA <span class="count">{{if .ASPA}}{{len .ASPA.Providers}}{{else}}0{{end}}</span></h2></div>
<h4 class="sub">{{call $t "aspa"}}</h4><p>{{if .ASPA}}{{range .ASPA.Providers}}<a href="/explorer?q=AS{{.}}">AS{{.}}</a> {{end}}{{else}}<span class="mut">{{call $t "no_aspa"}}</span>{{end}}</p>
<h4 class="sub">{{call $t "provider_of"}} <span class="mut">({{len .ProviderOf}})</span></h4><p>{{range .ProviderOf}}<a href="/explorer?q=AS{{.}}">AS{{.}}</a> {{else}}<span class="mut">{{call $t "none"}}</span>{{end}}</p></section>
<section class="card"><div class="card-h"><h2>{{call $t "sec_vrps"}} <span class="count">{{num $.Data.ASTotal}}</span></h2><span class="mut">{{if $.Data.ASCut}}{{call $t "truncated"}}{{end}}</span></div>
<div class="big">{{template "vrps" (rows $t .VRPs)}}</div></section>{{end}}
{{if .KeysWait}}<section class="card"><p class="mut">{{call $t "loading"}}</p></section>{{end}}
{{if and .Keys (not .AS)}}<section class="card"><div class="card-h"><h2>{{call $t "router_keys"}} <span class="count">{{.KeysTotal}}</span></h2></div>{{template "keystable" (kt $t .Keys)}}</section>{{end}}
{{if .ShowVRPs}}<section class="card"><div class="card-h"><h2>{{call $t "sec_vrps"}} <span class="count">{{.Total}}</span></h2><span class="mut">{{if or .Cut .Trunc}}{{call $t "truncated"}}{{end}}</span></div>
<div class="big">{{template "vrps" (rows $t .Rows)}}</div></section>
{{template "chains" (cc $t .Cards .CardCut)}}{{end}}{{end}}
{{template "foot" .}}{{end}}

{{define "short"}}{{if .L}}<h4>{{call .T .K}}</h4><table>{{range .L}}<tr><td>{{.ASN}}</td><td>{{.Prefix}}</td><td>maxLength {{.MaxLength}}</td></tr>{{end}}</table>{{end}}{{end}}
`

const templateText2 = `
{{define "history"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<p>{{range $h := list4}}<a href="/history?hours={{$h}}" {{if eq $h $.Data.Hours}}class="on"{{end}}>{{$h}}h</a> · {{end}} <span class="mut">{{call $t "history_help"}}</span></p>
<div class="card">{{.Chart}}</div>
{{if .Held}}<section class="card"><div class="card-h"><h2>{{call $t "held_n"}} <span class="count bad">{{len .Held}}</span></h2><span class="mut">{{call $t "held_help"}}</span></div>
<div class="wrap"><table><tr><th>{{call $t "time"}}</th><th class="n">serial</th><th class="n">VRP</th><th class="n">{{call $t "held_drop"}}</th><th>{{call $t "held_reason"}}</th></tr>
{{range .Held}}<tr><td>{{time .Start}}</td><td class="n"><a href="/changes?serial={{.Serial}}">{{.Serial}}</a></td><td class="n">{{num .VRPs}}</td><td class="n">{{if gt .Drop 0}}-{{num .Drop}}{{end}}</td><td>{{if .Reason}}{{.Reason}}{{else}}<span class="mut">-</span>{{end}}</td></tr>{{end}}</table></div></section>{{end}}
<div class="wrap"><table><tr><th>{{call $t "time"}}</th><th class="n">serial</th><th class="n">VRP</th><th class="n">ASPA</th><th class="n">{{call $t "added"}}</th><th class="n">{{call $t "removed"}}</th><th class="n">{{call $t "took"}}</th><th></th></tr>
{{range .Runs}}<tr><td>{{time .Start}}</td><td class="n"><a href="/changes?serial={{.Serial}}">{{.Serial}}</a></td><td class="n">{{num .VRPs}}</td><td class="n">{{num .VAPs}}</td><td class="n">+{{.Announced}}</td><td class="n">-{{.Withdrawn}}</td><td class="n">{{printf "%.0f" .DurationS}}s</td><td>{{if not .Applied}}<span class="b bad">{{call $t "not_applied"}}</span>{{end}}</td></tr>{{end}}</table></div>{{end}}
{{template "foot" .}}{{end}}

{{define "changes"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<div class="card pager"><span class="side">{{if .HasPrev}}<a href="/changes?serial={{.Prev}}">← {{call $t "older"}}</a>{{else}}<span class="off">← {{call $t "older"}}</span>{{end}}</span>
<span class="mid"><b>serial {{.Serial}}</b> <span class="mut">({{.Prev}} → {{.Serial}})</span>{{if .Time}}<br><span class="mut">{{.Time}}</span>{{end}}</span>
<span class="side r">{{if .HasNext}}<a href="/changes?serial={{inc .Serial}}">{{call $t "newer"}} →</a>{{else}}<span class="off">{{call $t "newer"}} →</span>{{end}}</span></div>
{{template "delta" (dl $t "added" .Added .AddN .AddCut)}}{{template "delta" (dl $t "removed" .Removed .RemN .RemCut)}}{{end}}
{{template "foot" .}}{{end}}

{{define "delta"}}<section class="card"><div class="card-h"><h2>{{call .T .K}} <span class="count">{{.N}}</span></h2><span class="mut">{{if .Cut}}{{call .T "truncated"}}{{end}}</span></div>
<div class="wrap"><table>{{range .Items}}<tr><td>{{.Type}}</td><td>{{if .ASN}}<a href="/explorer?q={{.ASN}}">{{.ASN}}</a>{{end}}</td><td>{{if .Prefix}}<a href="/explorer?q={{.Prefix}}">{{.Prefix}}</a>{{end}}</td><td>{{if .MaxLength}}maxLength {{.MaxLength}}{{end}}</td><td>{{if .Customer}}<a href="/explorer?q={{.Customer}}">{{.Customer}}</a>{{if .Providers}} → {{range $i, $p := .Providers}}{{if $i}}, {{end}}<a href="/explorer?q={{$p}}">{{$p}}</a>{{end}}{{end}}{{end}}</td></tr>{{end}}</table></div></section>{{end}}
`

const templateText3 = `
{{define "chains"}}{{$t := .T}}{{if .Cards}}{{$n := len .Cards}}
{{range $i, $c := .Cards}}<section class="card"><div class="card-h"><h2>ROA {{inc $i}}<span class="of">/ {{$n}}</span> <span class="sep">·</span> {{asn $c.VRP.ASN}} <span class="sep">·</span> {{$c.VRP.Prefix}}-{{$c.VRP.MaxLength}} <span class="tag ta">{{$c.VRP.TA}}</span></h2>
<span class="mut">{{if $c.Expire}}{{call $t "vrp_expiry"}} : {{$c.Expire}}{{end}}</span></div>
{{if $c.Missing}}<p class="err">{{call $t "no_roa"}} {{$c.ROA}}</p>{{else}}
<h4 class="sub">ROA</h4><div class="roa">{{template "kvtable" $c.ROADetail}}</div>
<h4 class="sub">{{call $t "sub_chain"}}</h4>{{if $c.NoTA}}<p class="warn">{{call $t "no_ta"}}</p>{{end}}
<div class="wrap"><table class="chain"><tr><th>{{call $t "step"}}</th><th>{{call $t "state"}}</th><th>Path</th><th>{{call $t "expires"}}</th><th>{{call $t "content"}}</th></tr>
{{range $c.Steps}}<tr class="tg{{if .Muted}} mut{{end}}" title="{{call $t "click_detail"}}"><td style="padding-left:{{pad .Depth}}px">{{if .Depth}}{{if .Muted}}├ {{else}}└ {{end}}{{end}}{{.Name}}</td>
<td><span class="b {{if eq .O.Status "valid"}}valid{{else}}invalid{{end}}">{{.O.Status}}</span>{{if .O.Reason}} {{.O.Reason}}{{end}}</td>
<td class="pt">{{if eq .O.Type "cer"}}<a class="mono" href="/ca?uri={{.O.URI}}" title="{{.O.URI}}">{{cut .O.URI}}</a>{{else}}<a class="mono" href="/object?uri={{.O.URI}}" title="{{.O.URI}}">{{cut .O.URI}}</a>{{end}}</td>
<td>{{date .O.NotAfter}} <span class="mut">{{days .O.NotAfter}}</span>{{if .Earliest}}<span class="tag">{{call $t "earliest"}}</span>{{end}}</td><td class="mono">{{.Content}}</td></tr>
<tr hidden class="dt"><td colspan="5"><table class="kv">{{range .Detail}}<tr><th>{{.K}}</th><td>{{.V}}</td></tr>{{end}}</table></td></tr>{{end}}</table></div>{{end}}</section>{{end}}{{if .Cut}}<p class="mut">{{call $t "chain_cut"}}</p>{{end}}{{end}}{{end}}
`

const templateText4 = `
{{define "kvtable"}}<table class="kv">{{range .}}<tr><th>{{.K}}</th><td>{{.V}}</td></tr>{{end}}</table>{{end}}
{{define "object"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<form action="/object"><label>URI<input name="uri" value="{{.URI}}" size="90" placeholder="rsync://..."></label><button>{{call $t "search"}}</button></form>
{{if .Rows}}<div class="card">{{template "kvtable" .Rows}}</div><details><summary>JSON</summary><pre class="mono">{{.Raw}}</pre></details>{{end}}{{end}}
{{template "foot" .}}{{end}}

{{define "ca"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<form action="/ca"><label>CA URI<input name="uri" value="{{.URI}}" size="90" placeholder="rsync://...cer"></label><button>{{call $t "search"}}</button></form>
{{with .CA}}<div class="card">{{template "kvtable" $.Data.Info}}</div>
<div class="card"><b>{{call $t "counts"}}</b><table>{{range $.Data.Counts}}<tr><td>{{.K}}</td><td class="n">{{num .N}}</td></tr>{{end}}</table></div>
<div class="card"><b>{{call $t "resources"}}</b><div class="mono">{{$.Data.Res}}</div></div>
<h3>{{call $t "children"}} ({{num $.Data.NChild}})</h3>{{if $.Data.Cut}}<p class="mut">{{call $t "truncated"}}</p>{{end}}
<div class="wrap"><table><tr><th>{{call $t "type"}}</th><th>{{call $t "state"}}</th><th>Path</th></tr>{{range $.Data.Children}}<tr><td>{{.Type}}</td><td>{{.Status}}</td><td>{{.Link}}</td></tr>{{end}}</table></div>{{end}}{{end}}
{{template "foot" .}}{{end}}
`

const templateText5 = `
{{define "inav"}}{{$t := .T}}<div class="subnav">
<a href="/internal/" {{if eq .Sub "home"}}class="on"{{end}}>{{call $t "i_home"}}</a>
<a href="/internal/slurm" {{if eq .Sub "slurm"}}class="on"{{end}}>SLURM</a>
<a href="/internal/rtr" {{if eq .Sub "rtr"}}class="on"{{end}}>{{call $t "i_rtr"}}</a></div>{{end}}

{{define "onav"}}{{$t := .T}}<div class="subnav">
<a href="/repos" {{if eq .Sub "repos"}}class="on"{{end}}>{{call $t "i_repos"}}</a>
<a href="/rejects" {{if eq .Sub "rejects"}}class="on"{{end}}>{{call $t "i_rejects"}}</a>
<a href="/expiring" {{if eq .Sub "expiring"}}class="on"{{end}}>{{call $t "i_expiring"}}</a>
<a href="/objects" {{if eq .Sub "objects"}}class="on"{{end}}>{{call $t "i_objects"}}</a>
<a href="/unsafe" {{if eq .Sub "unsafe"}}class="on"{{end}}>{{call $t "i_unsafe"}}</a></div>{{end}}

{{define "i_home"}}{{template "head" .}}{{template "inav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_title"}}</h2><span class="mut">{{call $t "i_note"}}</span></div>
<div class="grid">
<a class="stat" href="/repos"><span>{{call $t "i_repos"}}</span><b>{{.Problems}}</b><span>{{call $t "i_problems"}}</span></a>
<a class="stat" href="/rejects"><span>{{call $t "i_rejects"}}</span><b>{{.Rejects}}</b></a>
<a class="stat" href="/unsafe"><span>{{call $t "i_unsafe"}}</span><b>{{.Unsafe}}</b></a>
<a class="stat" href="/internal/rtr"><span>{{call $t "i_rtr"}}</span><b>{{.Clients}}</b></a></div></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_repos"}}{{template "head" .}}{{template "onav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_repos"}} <span class="count">{{.Total}}</span></h2>
<span class="mut">{{call $t "i_problems"}} {{.Bad}} · {{if .OnlyProblems}}<a href="?all=1">{{call $t "i_showall"}}</a>{{else}}<a href="?">{{call $t "i_onlyprob"}}</a>{{end}}</span></div>
<div class="wrap"><table><tr><th>{{call $t "type"}}</th><th>{{call $t "state"}}</th><th>URI</th><th class="n">{{call $t "i_fail"}}</th><th>{{call $t "i_lastok"}}</th><th>{{call $t "i_next"}}</th><th>{{call $t "i_error"}}</th></tr>
{{range .Rows}}<tr><td>{{.Type}}</td><td><span class="b {{if eq .Result "ok"}}valid{{else if eq .Result "failed"}}invalid{{else}}unknown{{end}}">{{.Result}}</span>{{if .Skipped}} <span class="tag">skip</span>{{end}}</td>
<td class="mono">{{.URI}}</td><td class="n">{{.Failures}}</td><td>{{ago .LastOK}}</td><td>{{ago .NextTry}}</td><td class="mono err-cell">{{.Error}}</td></tr>{{end}}</table></div></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_rejects"}}{{template "head" .}}{{template "onav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_rejects"}} <span class="count">{{len .Rows}}</span></h2></div>
<form><label>{{call $t "i_filter"}}<input name="q" value="{{.Filter}}" size="40" placeholder="expired, cloudie ..."></label><button>{{call $t "search"}}</button></form>
<h4 class="sub">{{call $t "i_reasons"}}</h4><table>{{range .Reasons}}<tr><td><a href="?q={{.K}}">{{.K}}</a></td><td class="n">{{.N}}</td></tr>{{end}}</table></section>
<section class="card"><div class="card-h"><h2>{{call $t "i_list"}}</h2></div><div class="wrap"><table><tr><th>URI</th><th>{{call $t "i_reason"}}</th></tr>
{{range .Rows}}<tr><td class="mono"><a href="/object?uri={{.URI}}">{{.URI}}</a></td><td>{{.Reason}}</td></tr>{{end}}</table></div></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_expiring"}}{{template "head" .}}{{template "onav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_expiring"}} <span class="count">{{num .Total}}</span></h2><span class="mut">{{if .Cut}}{{call $t "truncated"}}{{end}}</span></div>
<form><label>{{call $t "i_days"}}<select name="days">{{range $d := list7}}<option value="{{$d}}" {{if eq $d $.Data.Days}}selected{{end}}>{{$d}}</option>{{end}}</select></label>
<label>{{call $t "type"}}<select name="type">{{range $x := list5}}<option value="{{$x}}" {{if eq $x $.Data.Type}}selected{{end}}>{{if $x}}{{$x}}{{else}}{{call $t "m_any"}}{{end}}</option>{{end}}</select></label>
<label>{{call $t "asn"}}<input name="as" value="{{.ASN}}" size="12" placeholder="AS64496"></label><button>{{call $t "search"}}</button></form></section>
<section class="card"><div class="wrap"><table><tr><th>{{call $t "type"}}</th><th>{{call $t "expires"}}</th><th>{{call $t "i_content"}}</th><th>TA</th><th>URI</th></tr>
{{range .Rows}}<tr><td>{{.O.Type}}</td><td>{{date .O.NotAfter}} <span class="mut">{{.Left}}</span></td><td>{{if eq .O.Type "roa"}}<a href="/explorer?q=AS{{.O.Summary.ASN}}">AS{{.O.Summary.ASN}}</a> {{range .O.Summary.Prefixes}}{{.Prefix}}-{{.MaxLength}} {{end}}{{else}}{{.O.Summary.Kind}}{{end}}</td><td>{{.O.TA}}</td>
<td class="mono"><a href="/object?uri={{.O.URI}}">{{cut .O.URI}}</a></td></tr>{{end}}</table></div></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_unsafe"}}{{template "head" .}}{{template "onav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_unsafe"}} <span class="count">{{.Total}}</span></h2></div><p class="mut">{{call $t "i_unsafe_help"}}</p>
<div class="wrap"><table><tr><th>{{call $t "prefix"}}</th><th>{{call $t "max_length"}}</th><th>{{call $t "asn"}}</th><th>TA</th><th>{{call $t "i_rejca"}}</th></tr>
{{range .VRPs}}<tr><td><a href="/explorer?prefix={{.Prefix}}">{{.Prefix}}</a></td><td>{{.MaxLength}}</td><td><a href="/explorer?q=AS{{.ASN}}">{{asn .ASN}}</a></td><td>{{.TA}}</td><td class="mono"><a href="/object?uri={{.RejectedCA}}">{{cut .RejectedCA}}</a></td></tr>{{end}}</table></div></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_slurm"}}{{template "head" .}}{{template "inav" .}}{{$t := .T}}
<section class="card"><div class="card-h"><h2>SLURM</h2></div><pre class="mono">{{.Data}}</pre></section>
{{template "foot" .}}{{end}}

{{define "i_rtr"}}{{template "head" .}}{{template "inav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_rtr"}} <span class="count">{{len .RTR}}</span></h2><span class="mut">{{.Version}}</span></div>
<div class="wrap"><table><tr><th>{{call $t "i_remote"}}</th><th>{{call $t "i_ver"}}</th><th>{{call $t "i_since"}}</th><th>{{call $t "i_lastq"}}</th><th class="n">serial</th><th>{{call $t "i_trusted"}}</th></tr>
{{range .RTR}}<tr><td class="mono">{{.Remote}}</td><td>v{{.Version}} {{.Transport}}</td><td>{{time .Since}}</td><td>{{ago .LastQuery}}</td><td class="n">{{.Serial}}</td><td>{{if .Trusted}}✔{{end}}</td></tr>{{end}}</table></div></section>
<section class="card"><div class="card-h"><h2>{{call $t "i_handover"}}</h2></div><table class="kv">
<tr><th>count</th><td>{{.Handover.Count}}</td></tr><tr><th>last</th><td>{{if .Handover.LastAt}}{{time .Handover.LastAt}}{{end}}</td></tr><tr><th>from</th><td>{{.Handover.From}}</td></tr>
<tr><th>pause</th><td>{{.Handover.PauseMS}} ms</td></tr><tr><th>rtr conns</th><td>{{.Handover.Conns}}</td></tr>{{if .Handover.Err}}<tr><th>error</th><td>{{.Handover.Err}}</td></tr>{{end}}</table></section>{{end}}
{{template "foot" .}}{{end}}

{{define "i_objects"}}{{template "head" .}}{{template "onav" .}}{{$t := .T}}{{with .Data}}
<section class="card"><div class="card-h"><h2>{{call $t "i_objects"}}{{if .Rows}} <span class="count">{{num .Total}}</span>{{end}}</h2></div>
<form><label>{{call $t "type"}}<select name="type">{{range $x := list9}}<option value="{{$x}}" {{if eq $x $.Data.Type}}selected{{end}}>{{if $x}}{{$x}}{{else}}{{call $t "m_any"}}{{end}}</option>{{end}}</select></label>
<label>{{call $t "state"}}<select name="status">{{range $x := list3}}<option value="{{$x}}" {{if eq $x $.Data.Status}}selected{{end}}>{{if $x}}{{$x}}{{else}}{{call $t "m_any"}}{{end}}</option>{{end}}</select></label>
<label>TA<select name="ta">{{range $x := list6}}<option value="{{$x}}" {{if eq $x $.Data.TA}}selected{{end}}>{{if $x}}{{$x}}{{else}}{{call $t "m_any"}}{{end}}</option>{{end}}</select></label>
<label>{{call $t "asn"}}<input name="as" value="{{.AS}}" size="12" placeholder="AS64496"></label><label>{{call $t "prefix"}}<input name="prefix" value="{{.Prefix}}" size="20" placeholder="192.0.2.0/24"></label>
<label>URI ({{call $t "i_prefix_match"}})<input name="uri" value="{{.URI}}" size="40" placeholder="rsync://..."></label><button>{{call $t "search"}}</button></form>
<p class="mut note">{{call $t "i_catree"}} {{range .TAs}}<a href="/ca?uri={{.URI}}"><b>{{.Name}}</b></a> {{end}}</p></section>
{{if .Rows}}<section class="card"><div class="card-h"><h2>{{call $t "i_list"}}</h2><span class="mut">{{inc .Page}} / {{.Pages}}</span></div>
{{template "pagenav" .Bar}}<div class="wrap"><table><tr><th>{{call $t "type"}}</th><th>{{call $t "state"}}</th><th>TA</th><th>{{call $t "i_content"}}</th><th>{{call $t "expires"}}</th><th>URI</th><th>{{call $t "i_parent"}}</th></tr>
{{range .Rows}}<tr><td>{{.O.Type}}</td><td><span class="b {{if eq .O.Status "valid"}}valid{{else}}invalid{{end}}">{{.O.Status}}</span>{{if .O.Reason}} {{.O.Reason}}{{end}}</td><td>{{.O.TA}}</td><td>{{.Summary}}</td><td>{{date .O.NotAfter}}</td><td class="pt">{{.Link}}</td><td class="pt">{{.Parent}}</td></tr>{{end}}</table></div>{{template "pagenav" .Bar}}</section>{{end}}{{end}}
{{template "foot" .}}{{end}}
`

const templateText6 = `
{{define "pagenav"}}{{if .}}<nav class="pgn" aria-label="pages">{{if .Prev}}<a href="{{.Prev}}" aria-label="prev">‹</a>{{else}}<span class="off">‹</span>{{end}}{{range .Nav}}{{if .Gap}}<span class="gap">…</span>{{else if .Cur}}<span class="cur">{{.N}}</span>{{else}}<a href="{{.URL}}">{{.N}}</a>{{end}}{{end}}{{if .Next}}<a href="{{.Next}}" aria-label="next">›</a>{{else}}<span class="off">›</span>{{end}}
<form class="pgj" method="get"><input type="number" name="pg" min="1" max="{{.Pages}}" placeholder="{{inc .Page}} / {{.Pages}}" aria-label="page" inputmode="numeric" required>{{range .Keep}}<input type="hidden" name="{{.K}}" value="{{.V}}">{{end}}<button>{{.Go}}</button></form></nav>{{end}}{{end}}

{{define "keystable"}}{{$t := .T}}<div class="wrap"><table><tr><th>{{call $t "asn"}}</th><th>SKI</th><th>{{call $t "pubkey"}}</th></tr>
{{range .Keys}}<tr><td><a href="/explorer?q=AS{{.ASN}}">AS{{.ASN}}</a></td><td class="mono"><a href="/list?kind=keys&q={{.SKI}}">{{.SKI}}</a></td><td class="mono" title="{{.Pub}}">{{cutn .Pub 44}}</td></tr>{{end}}</table></div>{{end}}

{{define "list"}}{{template "head" .}}{{$t := .T}}{{with .Data}}
<div class="subnav"><a href="/list?kind=vrp" {{if eq .Kind "vrp"}}class="on"{{end}}>VRP</a><a href="/list?kind=aspa" {{if eq .Kind "aspa"}}class="on"{{end}}>ASPA</a><a href="/list?kind=keys" {{if eq .Kind "keys"}}class="on"{{end}}>{{call $t "router_keys"}}</a></div>
<section class="card"><div class="card-h"><h2>{{call $t "search"}}</h2></div>
<form><input type="hidden" name="kind" value="{{.Kind}}"><label class="grow">{{call $t "list_q"}}<input name="q" value="{{.Q}}" size="48" placeholder="{{if eq .Kind "vrp"}}192.0.2.0/24 · AS64496{{else if eq .Kind "aspa"}}AS64496{{else}}AS64496 · 62A7A7BC…{{end}}" autofocus></label>
{{if eq .Kind "vrp"}}<label>{{call $t "match"}}<select name="prefixMatch">
<option value="any" {{if eq .Match "any"}}selected{{end}}>{{call $t "m_any"}}</option><option value="exact" {{if eq .Match "exact"}}selected{{end}}>{{call $t "m_exact"}}</option>
<option value="lspec" {{if eq .Match "lspec"}}selected{{end}}>{{call $t "m_lspec"}}</option><option value="mspec" {{if eq .Match "mspec"}}selected{{end}}>{{call $t "m_mspec"}}</option></select></label>{{end}}
<button>{{call $t "search"}}</button></form></section>
{{if .Loading}}<section class="card"><p>{{call $t "loading"}}</p></section>{{else}}
<section class="card"><div class="card-h"><h2>{{if eq .Kind "vrp"}}VRP{{else if eq .Kind "aspa"}}ASPA{{else}}{{call $t "router_keys"}}{{end}} <span class="count">{{num .Total}}</span></h2><span class="mut">{{.Snapshot}}</span></div>
{{template "pagenav" .Bar}}
{{if eq .Kind "vrp"}}<div class="big"><div class="wrap"><table><tr><th>{{call $t "prefix"}}</th><th>{{call $t "max_length"}}</th><th>{{call $t "asn"}}</th></tr>
{{range .VRPs}}<tr><td><a href="/explorer?q={{.Prefix}}">{{.Prefix}}</a></td><td>{{.Max}}</td><td><a href="/explorer?q=AS{{.ASN}}">AS{{.ASN}}</a></td></tr>{{end}}</table></div></div>{{end}}
{{if eq .Kind "aspa"}}<div class="wrap"><table><tr><th>Customer</th><th>Providers</th></tr>
{{range .ASPAs}}<tr><td><a href="/explorer?q=AS{{.Customer}}">AS{{.Customer}}</a></td><td>{{range .Providers}}<a href="/explorer?q=AS{{.}}">AS{{.}}</a> {{else}}<span class="mut">AS0 ({{call $t "none"}})</span>{{end}}</td></tr>{{end}}</table></div>{{end}}
{{if eq .Kind "keys"}}{{template "keystable" (kt $t .Keys)}}{{end}}
{{template "pagenav" .Bar}}</section>{{end}}{{end}}
{{template "foot" .}}{{end}}
`
