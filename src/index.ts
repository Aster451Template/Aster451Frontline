interface Env {
	ASSETS: {
		fetch(request: Request): Promise<Response>;
	};
}

interface DocumentIndex {
	documents?: Array<{
		paths?: {
			html_path?: string;
		};
	}>;
}

const worker = {
	async fetch(request: Request, env: Env): Promise<Response> {
		const url = new URL(request.url);
		if (url.pathname !== "/") {
			return env.ASSETS.fetch(request);
		}

		const indexResponse = await env.ASSETS.fetch(new Request(new URL("/index.json", url)));
		if (!indexResponse.ok) {
			return new Response("Document index not found", { status: 404 });
		}

		const index = (await indexResponse.json()) as DocumentIndex;
		const htmlPath = index.documents?.[0]?.paths?.html_path;
		const filename = htmlPath?.split(/[\\/]/).pop();
		if (!filename || !/^[0-9a-f-]+\.html$/i.test(filename)) {
			return new Response("No generated document is available", { status: 404 });
		}

		const documentURL = new URL(`/${filename}`, url);
		return env.ASSETS.fetch(new Request(documentURL, request));
	},
};

export default worker;
