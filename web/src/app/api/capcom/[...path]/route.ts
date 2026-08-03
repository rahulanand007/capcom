import { NextRequest } from "next/server"

type RouteContext = {
  params: Promise<{
    path: string[]
  }>
}

const DEFAULT_CAPCOM_API_URL = "http://127.0.0.1:8081"

async function proxyCapcomRequest(request: NextRequest, context: RouteContext) {
  try {
    const { path } = await context.params
    const baseUrl = process.env.CAPCOM_API_URL ?? DEFAULT_CAPCOM_API_URL
    const upstreamUrl = new URL(path.join("/"), `${baseUrl.replace(/\/+$/, "")}/`)
    upstreamUrl.search = request.nextUrl.search

    const headers = new Headers()
    headers.set("Authorization", `Bearer ${process.env.CAPCOM_ADMIN_TOKEN ?? ""}`)

    const contentType = request.headers.get("content-type")
    if (contentType) {
      headers.set("Content-Type", contentType)
    }

    const accept = request.headers.get("accept")
    if (accept) {
      headers.set("Accept", accept)
    }

    const body =
      request.method === "GET" || request.method === "HEAD"
        ? undefined
        : await request.arrayBuffer()

    const upstream = await fetch(upstreamUrl, {
      method: request.method,
      headers,
      body,
    })

    const responseHeaders = new Headers()
    const upstreamContentType = upstream.headers.get("content-type")
    if (upstreamContentType) {
      responseHeaders.set("Content-Type", upstreamContentType)
    }

    const hasNoBody =
      request.method === "HEAD" ||
      upstream.status === 204 ||
      upstream.status === 205 ||
      upstream.status === 304

    return new Response(hasNoBody ? null : await upstream.arrayBuffer(), {
      status: upstream.status,
      headers: responseHeaders,
    })
  } catch {
    return Response.json(
      {
        error: {
          code: "SERVICE_UNAVAILABLE",
          message: "Capcom is temporarily unavailable. Try again shortly.",
          retryable: true,
        },
      },
      { status: 502 }
    )
  }
}

export function GET(request: NextRequest, context: RouteContext) {
  return proxyCapcomRequest(request, context)
}

export function POST(request: NextRequest, context: RouteContext) {
  return proxyCapcomRequest(request, context)
}

export function PATCH(request: NextRequest, context: RouteContext) {
  return proxyCapcomRequest(request, context)
}

export function DELETE(request: NextRequest, context: RouteContext) {
  return proxyCapcomRequest(request, context)
}

export function OPTIONS(request: NextRequest, context: RouteContext) {
  return proxyCapcomRequest(request, context)
}
