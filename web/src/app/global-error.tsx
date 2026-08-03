"use client"

export default function GlobalError({ reset }: { reset: () => void }) {
  return (
    <html lang="en">
      <body
        style={{
          minHeight: "100vh",
          margin: 0,
          display: "grid",
          placeItems: "center",
          background: "#0a0c10",
          color: "#e8ecf2",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        <main style={{ maxWidth: 520, padding: 32, textAlign: "center" }}>
          <div style={{ color: "#f2b441", fontSize: 12, letterSpacing: 2 }}>
            CAPCOM RECOVERY
          </div>
          <h1 style={{ fontSize: 22, margin: "14px 0 8px" }}>
            The console could not start
          </h1>
          <p style={{ color: "#8a93a2", lineHeight: 1.6 }}>
            Reload the console. If the problem continues, confirm the local
            Capcom services are running.
          </p>
          <button
            type="button"
            onClick={reset}
            style={{
              marginTop: 16,
              border: 0,
              borderRadius: 8,
              padding: "10px 16px",
              background: "#3de1a0",
              color: "#06231a",
              fontWeight: 700,
              cursor: "pointer",
            }}
          >
            Reload console
          </button>
        </main>
      </body>
    </html>
  )
}
