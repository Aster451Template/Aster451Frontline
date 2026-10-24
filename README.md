# Aster451 Frontline - Developing

Markdown の文書から HTML ページと JSON インデックスを生成する、Go 製の静的サイト構築ツールです。文書のメタデータには YAML 形式の Front Matter を使い、Git から文書ごとの変更履歴も取得します。

## 必要なもの

- Go 1.26.2 以降（`go.mod` に記載されたバージョン）
- Git。文書履歴の取得に使用します

## ビルドする

リポジトリのルートで実行します。

```sh
go mod download
go build ./...
go run .
```

`go run .` を実行すると、`public/` 以下の Markdown を処理し、HTML とインデックスを `dirc/` に生成します。既存の `dirc/index.json` があれば文書 ID を引き継ぎます。Git 履歴が取得できる文書は、最新コミット日時を更新日時として使います。

## Go テスト

GitHub Actions は Go の各パッケージを個別にテストします。実行ごとの結果はこの節に追記され、詳細ログは `log/` に保存されます。`python scripts/run_tests.py` で同じテストと記録をローカル実行できます。

## Cloudflare Workers にデプロイする

このプロジェクトは Cloudflare Workers の Static Assets を使います。`/` にアクセスすると `dirc/index.json` の先頭にある文書 HTML を返し、各 HTML と JSON インデックスも静的アセットとして配信します。`main` ブランチへ push すると、GitHub Actions が Go で HTML と JSON を生成し、Actions の実行成果物として `dirc/` を保存します。Cloudflare の API 認証情報は GitHub Actions では使いません。

```sh
npm install
npm run build
npm run dev
```

Worker のエントリーポイントとアセット設定は `src/index.ts` と `wrangler.toml` にあります。

## バグ・脆弱性の報告

[![バグ・不具合を報告](https://img.shields.io/badge/GitHub-%E3%83%90%E3%82%B0%E5%A0%B1%E5%91%8A-7057ff?logo=github)](https://github.com/Aster451Template/Aster451Frontline/issues/new?template=bug_report.yml) [![改善案を提案](https://img.shields.io/badge/GitHub-%E6%94%B9%E5%96%84%E6%A1%88%E3%82%92%E6%8F%90%E6%A1%88-2da44e?logo=github)](https://github.com/Aster451Template/Aster451Frontline/issues/new?template=feature_request.yml) [![脆弱性を非公開で報告](https://img.shields.io/badge/Security-%E9%9D%9E%E5%85%AC%E9%96%8B%E5%A0%B1%E5%91%8A-critical?logo=github)](https://github.com/Aster451Template/Aster451Frontline/security/advisories/new)

バグ報告フォームでは、影響に応じて重要度を選べます。

| 評価 | 目安 |
| --- | --- |
| `notice` | 軽微な表示・文書上の問題 |
| `caution` | 影響が限定的で、回避策がある |
| `warning` | 主要な機能に影響する |
| `danger` | 広範な機能停止やデータ破損のおそれがある |
| `critical` | サービス停止、重大なデータ損失など緊急対応が必要 |

脆弱性の詳細や再現手順は公開 Issue に書かず、GitHub Security Advisory の非公開報告リンクを使ってください。このリンクを利用できるよう、リポジトリ管理者は GitHub の Settings → Advanced Security で Private vulnerability reporting を有効にしてください。

## 文書を追加する

1. `public/` 以下に `.md` ファイルを作成します。サブディレクトリも使えます。
2. ファイルの先頭に、`=====` で囲んだ YAML Front Matter を記述します。
3. 区切りの後に Markdown 本文を書きます。
4. `go run .` を実行します。

```markdown
=====
title: 駅の案内システム
path: /docs/station-guide
author: Aster451 Project
tags:
  - 駅
  - 案内
created_at: 2026-10-12T00:00:00Z
updated_at: 2026-10-12T00:00:00Z
=====

# 駅の案内システム

ここに文書を書きます。
```

`title`、`path`、`author` は必須です。`id` は省略すると UUID が生成されます。`tags` は任意の文字列リストです。`created_at` を省略すると実行時の UTC 時刻が入り、`updated_at` を省略すると作成日時が使われます。Git 履歴がある場合、ビルド時に `updated_at` は最新コミット日時で上書きされます。

## 処理の流れ

1. `public/` を再帰的に走査して Markdown ファイルを探します。
2. Front Matter と本文を分離し、必須メタデータを確認します。
3. Git 履歴を `git log --follow` で取得します。
4. Markdown を Goldmark で HTML に変換し、`template/document.html` に適用します。
5. `dirc/index.json` と `dirc/tags.json` を更新します。
6. 原稿が削除されていれば、対応する生成 HTML も削除します。

既存インデックスに記録された最新コミットと現在の最新コミットが同一で、生成 HTML も残っている文書は HTML の再生成を省略します。文書の同一性にはリポジトリ内の原稿パスの SHA-256 を使い、同じパスの文書では既存 UUID を維持します。

## 生成物

| パス | 内容 |
| --- | --- |
| `dirc/<UUID>.html` | 文書ごとの HTML ページ |
| `dirc/index.json` | 生成日時、文書メタデータ、原稿・HTML パス、Git 履歴を含む一覧 |
| `dirc/tags.json` | タグごとに関連文書 UUID をまとめた一覧 |

インデックスとタグ一覧は整形済み JSON です。タグと UUID は安定した順序で出力され、同一文書内の重複タグは除去されます。

## ディレクトリ構成

```text
main.go                  ビルド処理のエントリーポイント
public/                  Markdown 原稿
parser/                  Front Matter の解析と検証
builder/                 HTML 生成、JSON インデックス、Git 履歴
models/                  文書・履歴・インデックスのデータ型
utils/                   原稿パスのハッシュ化
template/document.html   文書 HTML テンプレート
dirc/                    生成物
```

`dirc/` は生成先です。必要な成果物を残した上で、直接編集した変更が次のビルドで維持されることは保証されません。

## 注意点

- ビルドには Git コマンドが必要です。Git 履歴のない原稿では履歴が空になり、Front Matter の `updated_at`（または `created_at`）が使われます。
- Markdown から生成した HTML はテンプレートへそのまま渡されます。`template.HTML` によって HTML エスケープを無効にしているため、信頼できない原稿を処理する場合はサニタイズを追加してください。
- 設定値（原稿ディレクトリ、出力先、テンプレート）は現在 `main.go` の定数で指定されています。
- `1.go` と `2.go` には別の型定義・処理実装が含まれています。現行の `go run .` の処理は `main.go` と `parser/`、`builder/`、`models/`、`utils/` の実装を通ります。

## 今後の展望

次の改善を検討できます。これらは現在の実装に含まれていない計画です。

- 原稿ディレクトリ、出力先、テンプレートをコマンドライン引数または設定ファイルで指定できるようにする。
- Front Matter の形式、パスの重複、日付の妥当性を検証し、エラー箇所を原稿ファイル名と行番号で示す。
- Git 履歴がない環境でもビルドできるモードを追加し、コミット履歴取得と HTML 生成を独立させる。
- 信頼できない Markdown 入力に備え、生成 HTML のサニタイズとテンプレート出力の安全性を強化する。
- ビルド結果の再現性を高め、未変更ファイルの判定をコミット SHA だけでなく原稿内容やテンプレートの変更も考慮する。
- パーサー、履歴取得、生成物の更新・削除を自動テストし、CI で `go test ./...` とビルドを確認する。
- 文書数が増えた際に、並列生成や実行時間・生成件数のレポートを検討する。

## ライセンス

MIT License

## CI test results

### 20261024T052204Z-local-1 UTC

| Package | Result | Log |
| --- | --- | --- |
| `.` | **PASS** | [go-test-20261024T052204Z-local-1-root.log](log/go-test-20261024T052204Z-local-1-root.log) |
| `./builder` | **PASS** | [go-test-20261024T052204Z-local-1-builder.log](log/go-test-20261024T052204Z-local-1-builder.log) |
| `./parser` | **PASS** | [go-test-20261024T052204Z-local-1-parser.log](log/go-test-20261024T052204Z-local-1-parser.log) |
| `./models` | **PASS** | [go-test-20261024T052204Z-local-1-models.log](log/go-test-20261024T052204Z-local-1-models.log) |
| `./utils` | **PASS** | [go-test-20261024T052204Z-local-1-utils.log](log/go-test-20261024T052204Z-local-1-utils.log) |
