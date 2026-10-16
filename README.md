# Aster451 Frontline

Markdown で文書を書き、HTML と検索データを自動生成して公開する、小さな静的サイト基盤です。文書を編集する場所、生成処理、検索 API の役割を分けています。

## まず動かす

必要なものは Node.js 22 以降と npm です。

```bash
npm install
npm run build
npm test
npm run typecheck
```

生成物を作り、Workerをターミナルで起動します。

```bash
npm run build
npm run dev:worker
```

別のターミナルで静的サイトを起動します。

```bash
npm run dev
```

ブラウザーで `http://127.0.0.1:8788` を開くとページを見られます。検索 API は `http://127.0.0.1:8787` で動きます。PowerShell で `$env:SEARCH_API_URL = "http://127.0.0.1:8787"` を設定してから `npm run dev` を実行すると、検索結果ページがローカル Worker を呼び出します。

Worker は Pages の JSON を `wrangler.toml` の `PAGES_BASE_URL`（既定値 `http://127.0.0.1:8788`）から読み込みます。`npm run dev` は静的サイトをビルドしてPagesを起動し、`npm run dev:worker` は検索 Worker だけを起動します。公開環境では `SEARCH_API_URL` を空のままにすると、現在のサイトと同じオリジンの `/api/search` を呼び出します。

## 文書を追加する

1. `public/` の中に `.md` ファイルを作ります。サブフォルダーを使って整理できます。
2. ファイル先頭に YAML Front Matter を書きます。
3. 本文を Markdown で書きます。
4. `npm run build` を実行します。
5. `templates/` の HTML と `data/` の JSON が更新されたことを確認します。

```markdown
---
title: "駅の案内システム"
path: "/docs/station-guide"
author: "Aster451 Project"
tags:
  - 駅
  - 案内
description: "駅の案内システムについて説明します。"
created_at: "2026-10-12T00:00:00Z"
updated_at: "2026-10-12T00:00:00Z"
---

# 駅の案内システム

ここに文書を書きます。
```

### Front Matter の項目

| 項目 | 必須 | 説明 |
| --- | --- | --- |
| `title` | はい | ページのタイトル |
| `path` | はい | `/` から始まる公開 URL。例: `/docs/station-guide` |
| `author` | はい | 作者または担当者 |
| `tags` | いいえ | タグのリスト。複数のタグ条件はすべて一致する必要があります |
| `description` | いいえ | 検索結果や検索エンジンに表示する説明。未指定なら本文から生成します |
| `created_at` | はい | 作成日時。UTC の ISO 8601 形式 |
| `updated_at` | いいえ | 更新日時。UTC の ISO 8601 形式。未指定時は作成日時 |

公開パスが重複している場合、必須項目がない場合、日時が不正な場合はビルドがエラーになります。日時は原稿に明示して管理してください。HTML の出力先は URL から決まり、`/` は `templates/index.html`、`/docs/station-guide` は `templates/docs/station-guide.html` になります。

## フォルダーの見方

```text
public/                  編集する原稿、CSS、ブラウザー用 JavaScript
scripts/                 Markdown の検証、HTML 生成、JSON インデックス生成
layouts/layout.html      すべてのページで使う HTML のひな形
layouts/search.html     自動生成する検索ページの HTML ひな形
templates/               ビルドで生成される HTML
data/                    ページ情報と全文検索用 JSON
src/                     Cloudflare Worker の検索 API
tests/                   検索処理のテスト
.github/workflows/       GitHub Actions による生成の自動化
```

原稿を直すときは `public/` を編集します。`templates/` と `data/` は生成物なので、直接編集せず `npm run build` で作り直してください。HTML の共通レイアウトは `layouts/layout.html` で変更できます。

## 検索のしくみ

各ページの検索欄に語句を入力すると、検索結果は `/search?q=...` の別ページに表示します。ビルド時にタイトル・パス・作者・タグ・本文を NFKC 正規化し、英字を小文字に揃えて Bi-gram（隣り合う 2 文字）へ分割します。検索時も同じ正規化を行います。すべての検索語とフィルターに一致したページだけが返ります。1 文字の検索語は Bi-gram を作れないため、検索対象テキストを直接確認します。

検索データは次の 3 ファイルです。

- `data/pages.json`: ページのメタデータ
- `data/search-index.json`: Bi-gram からページ ID を引く転置インデックス
- `data/search-documents.json`: 候補が検索語を実際に含むか確かめる正規化済みテキスト

## 検索 API

Worker のエンドポイントは `GET /api/search` です。複数条件は AND で組み合わせます。

```text
/api/search?q=列車案内
/api/search?tag=列車&tag=案内
/api/search?author=Aster451%20Project
/api/search?q=列車&updated_after=2026-10-01T00:00:00Z
```

| パラメーター | 説明 |
| --- | --- |
| `q` | 全文検索語。空白や句読点で複数語に分け、すべてを含むページを返します |
| `tag` | タグ。複数指定またはカンマ区切りにできます |
| `author` | 作者の完全一致（英字は大小文字を区別しません） |
| `created_after`, `created_before` | 作成日時の範囲。UTC ISO 8601 形式 |
| `updated_after`, `updated_before` | 更新日時の範囲。UTC ISO 8601 形式 |
| `limit` | 1〜100 件。既定値は 20 |
| `offset` | 0〜10000 件目から。既定値は 0 |

成功時は `{ "total": 1, "results": [...] }` を返します。入力が不正な場合は HTTP 400 と `{ "error": "..." }` を返します。既定の並び順は更新日時の新しい順です。

## HTML の安全性

Markdown の通常の記法は使えますが、原稿に直接書いた HTML は出力時に取り除きます。リンクや画像は `javascript:` など実行可能な URL を無効にします。サイトの HTML ひな形へ値を埋め込む箇所では、文字列を HTML エスケープします。

## GitHub Actions

`.github/workflows/markdown-build.yml` は `main` ブランチへの原稿・生成処理関連ファイルの push と、手動実行で動きます。型チェックとテストの後に HTML、検索 JSON、サイトマップ、robots.txt を生成します。出力に差分がある場合だけ生成物をコミットします。ボットのコミットは監視対象パスに含まれないため、生成を繰り返しません。

このリポジトリを初めて GitHub に push する場合は、GitHub 側の既定ブランチ名を `main` に合わせてください。Pages が GitHub Actions からデプロイされた出力を配信するよう、Pages のビルド／デプロイ設定もリポジトリの構成に合わせて設定します。

## Cloudflare Pages と Worker の設定

1. GitHub リポジトリの Settings → Secrets and variables → Actions で `CLOUDFLARE_API_TOKEN` secret、`CLOUDFLARE_ACCOUNT_ID` variable、`CLOUDFLARE_PAGES_PROJECT` variable を設定します。API token には Pages のデプロイ権限を付与します。
2. ワークフローは `templates/` の HTML、`public/assets/` の CSS と JavaScript、`data/`、サイトマップを `_site/` にまとめて Pages へデプロイします。HTML ファイル名は Cloudflare Pages の Clean URLs により拡張子のない公開 URL に対応します。
3. Cloudflare Pages プロジェクトを先に作成し、プロジェクト名を `CLOUDFLARE_PAGES_PROJECT` に設定します。Git 連携による自動ビルドと二重管理しないよう、この方式では GitHub Actions が Pages のデプロイを担当します。
4. Worker をデプロイする前に `wrangler.toml` の `PAGES_BASE_URL` を Pages の実際のオリジンに変更します。
5. `npm run deploy:worker` を実行して Worker をデプロイします。Worker のルートを Pages と同じホストの `/api/search*` に割り当てます。

Worker は Pages 上の 3 つの JSON を HTTP で読み込みます。開発用には `PAGES_BASE_URL` の既定値 `http://127.0.0.1:8788` を使います。公開構成では必ず実際の Pages URL に置き換えてください。大規模なコンテンツを扱う場合は、JSON の転送量や Worker の実行時間を測り、データ配置を見直します。

## コマンド一覧

| コマンド | 用途 |
| --- | --- |
| `npm run build` | HTML・検索 JSON・サイトマップを生成し、不要な HTML を削除 |
| `npm run build:check` | 生成物に差分がないか確認 |
| `npm run typecheck` | TypeScript の型を確認 |
| `npm test` | 検索の単体テストを実行 |
| `npm run dev` | Cloudflare Pages の静的サイトをローカル実行 |
| `npm run dev:worker` | 検索 Worker をローカル実行 |
| `npm run deploy:worker` | Worker を Cloudflare にデプロイ |

## 公開前に設定する値

- `SITE_URL`: ビルド時の公開サイト URL。canonical、サイトマップ、robots.txt に使います。未設定時はサンプル用 URL です。
- `wrangler.toml` の `PAGES_BASE_URL`: Worker が検索 JSON を取得する Pages のオリジン。

例: `SITE_URL=https://example.com npm run build`。Windows PowerShell では `$env:SITE_URL = "https://example.com"; npm run build` とします。
