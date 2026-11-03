# report-studio-mcp

エージェントが書いた Markdown のレポート原稿（文章、グラフのデータ、構成図の定義）を受け取り、
組版した PDF を呼び出し側の作業ディレクトリに出力する MCP サーバです。外部のサービスやブラウザには
頼りません。組版は macOS 自身の機能（Core Text・Core Graphics）で行い、日本語の禁則処理が効き、
フォントは使った文字だけが埋め込まれます。

[English README](README.md)

## なぜ

gem-agent や lagent で調べた結果は、Markdown の応答として手元に残ります。人に渡すには手で
体裁を整える必要があり、グラフや構成図を入れるのはさらに手間がかかります。report-studio-mcp は、
エージェントが書いた原稿から、そのまま渡せる文書を作ります。

## 受け付けるもの

- **普通の Markdown**: CommonMark 0.31.2 に、GitHub の表・取り消し線・チェックリスト・自動リンク、
  脚注、注記（`> [!NOTE]` など）を加えたもの。任意の front matter で、題名・副題・作成者・日付と、
  全ページに印字する区分（「社外秘」など）を指定できます。
- **データからのグラフ**: ` ```chart ` ブロックに JSON を書くと、サーバが描きます。棒（縦・横、
  グループ・積み上げ）、折れ線、散布図。
- **構成図**: ` ```mermaid ` ブロック（flowchart、sequence、ER、state、gantt、pie、mindmap）を
  [mermaid-render](https://github.com/nlink-jp/mermaid-render) で描きます。描けない図はソースを
  載せて警告します。
- **画像**: 作業ディレクトリの中の PNG・JPEG ファイル。

エラーにするのは、出力が間違ったものになる場合（グラフの JSON の誤り、画像が無い、上限超過）だけです。
それ以外は、原稿の行番号つきの警告として返します。詳しい使い方はサーバの `get_usage` ツールにあります。

## ツール

| ツール | 役割 |
|---|---|
| `get_usage` | 使い方の全体（原稿の書き方、グラフの JSON、上限、エラーと警告） |
| `render` | `{work_dir, workspace_id, source}` → `<work_dir>/<workspace_id>/out/<原稿のパス>.pdf`。ページ数と警告も返す |

`work_dir` はすべての呼び出しで必須で、既定値はありません。呼び出し側が読み戻せる絶対パスの
ディレクトリを渡します（組織 ADR-021）。

## 安全のための設計

- 読み書きはすべて作業ディレクトリの `os.Root` を通します。開いたファイルはカーネルが示す実際のパスで
  [pathguard](https://github.com/nlink-jp/pathguard) の床に照らします。読むのは通常のファイルだけで、
  出力は新しい一時名で書いてから名前を付け替えます。
- クリックできるリンクは `http`・`https`・`mailto` だけです。無害化された指標は書かれたまま出します。
- PDF の文書情報に入れるのは題名と作成者だけです。

## 動作環境

Apple Silicon の macOS 26 以降。上に挙げた nlink-jp のモジュールのほかに外部の依存はありません。

## ビルド

```bash
make build    # → dist/report-studio-mcp
make test
```

## 設計

要件と設計判断は RFP（[docs/ja/report-studio-mcp-rfp.ja.md](docs/ja/report-studio-mcp-rfp.ja.md)）、
実装前の実測は [docs/ja/phase0-criteria.ja.md](docs/ja/phase0-criteria.ja.md) にあります。

## ライセンス

MIT — [LICENSE](LICENSE) を参照。試験データとして使っている CommonMark・GFM 仕様の例は
CC BY-SA 4.0 です（`internal/markdown/testdata/NOTICE`）。
