# report-studio-mcp

エージェントが書いた Markdown のレポート原稿（文章、グラフのデータ、構成図の定義）を受け取り、
組版した PDF と編集できる Word 文書（.docx）を呼び出し側の作業ディレクトリに出力する MCP サーバです。
外部のサービスやブラウザには頼りません。組版は macOS 自身の機能（Core Text・Core Graphics）で行います。

[English README](README.md)

## なぜ

gem-agent や lagent で調べた結果は、Markdown の応答として手元に残ります。人に渡すには手で
体裁を整える必要があり、グラフや構成図を入れるのはさらに手間がかかります。report-studio-mcp は、
エージェントが書いた原稿から、そのまま渡せる文書を作ります。

## 設計

要件と設計判断は RFP にあります:
[docs/ja/report-studio-mcp-rfp.ja.md](docs/ja/report-studio-mcp-rfp.ja.md)

- 入力は普通の Markdown（CommonMark と GitHub の拡張）、出力は PDF と .docx。
- データからのグラフ（` ```chart `）と、mermaid の図（` ```mermaid `）。
- 出力の見た目は設定のプロファイルで切り替え。
- macOS（Apple Silicon）専用。外部の依存はありません。

## ビルド

```bash
make build    # → dist/report-studio-mcp
make test
```

## ライセンス

MIT — [LICENSE](LICENSE) を参照。
