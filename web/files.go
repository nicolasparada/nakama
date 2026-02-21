package web

import "embed"

//go:embed all:dist
var StaticFiles embed.FS

//go:embed template/*
var TemplateFiles embed.FS
