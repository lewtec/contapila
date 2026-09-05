module github.com/lucasew/contapila-go

go 1.27.0

require (
	cuelang.org/go v0.17.0
	github.com/a-h/templ v0.3.1020
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/dslipak/pdf v0.0.2
	github.com/lewtec/eletrocromo v0.0.0-20260720233412-019f2474a08f
	github.com/mattn/go-isatty v0.0.20
	github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar v0.0.0
	github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/beancount v0.0.0-20260713221032-8673315d25fc
	github.com/spf13/cobra v1.10.2
	github.com/xuri/excelize/v2 v2.11.0
	go.lsp.dev/jsonrpc2 v1.0.1
	go.lsp.dev/protocol v1.0.1
	go.lsp.dev/uri v1.0.1
	golang.org/x/sync v0.21.0
)

require (
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/dop251/goja v0.0.0-20260903201622-f87b40ad7341 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/proto v1.14.3 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lewtec/tailgopher v0.0.0-20260905000803-9acf23e062c8 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/go v0.0.0-20260713221032-8673315d25fc // indirect
	github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/json v0.0.0-20260713221032-8673315d25fc // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/protocolbuffers/txtpbfmt v0.0.0-20260420112717-c39628bde8b5 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.7 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar => github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar v0.0.0-20260713221032-8673315d25fc

tool (
	github.com/a-h/templ/cmd/templ
	github.com/lewtec/tailgopher/cmd/tailwind
)
