from fastapi.testclient import TestClient

from m365_mcp import __main__ as server_main
from m365_mcp.app import app


def test_health_endpoint() -> None:
    with TestClient(app) as client:
        response = client.get("/health")
        healthz_response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {
        "status": "ok",
        "service": "m365-mcp-server",
        "version": "0.1.2",
    }
    assert healthz_response.status_code == 200
    assert healthz_response.json() == response.json()


def test_mcp_mount_is_present() -> None:
    assert any(getattr(route, "path", None) == "/mcp" for route in app.routes)


def test_production_entrypoint_disables_request_line_access_logs(
    monkeypatch,
) -> None:
    invoked: dict[str, object] = {}

    def fake_run(application, **kwargs) -> None:
        invoked["application"] = application
        invoked.update(kwargs)

    monkeypatch.setattr(server_main.uvicorn, "run", fake_run)
    monkeypatch.setenv("MCP_HOST", "127.0.0.1")
    monkeypatch.setenv("MCP_PORT", "8123")

    server_main.main()

    assert invoked == {
        "application": server_main.app,
        "host": "127.0.0.1",
        "port": 8123,
        "access_log": False,
    }
