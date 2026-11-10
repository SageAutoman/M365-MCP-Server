import json
from pathlib import Path
import re


ROOT = Path(__file__).resolve().parents[1]
CONNECTOR = ROOT / "workbuddy"


def _frontmatter(path: Path) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    match = re.match(r"---\r?\n(?P<body>.*?)\r?\n---\r?\n", text, re.DOTALL)
    assert match is not None, f"missing YAML frontmatter: {path}"
    values: dict[str, str] = {}
    for line in match.group("body").splitlines():
        key, separator, value = line.partition(":")
        assert separator and key and value.strip(), f"invalid frontmatter line: {line}"
        values[key.strip()] = value.strip()
    return values


def test_connector_uses_official_oauth_package_shape_without_credentials() -> None:
    metadata = json.loads((CONNECTOR / "connector-meta.json").read_text("utf-8"))
    configuration = json.loads((CONNECTOR / "mcp.json").read_text("utf-8"))

    assert metadata["source"] == "sagehou-m365-mcp-server"
    assert re.fullmatch(r"[a-z0-9-]+", metadata["source"])
    assert metadata["type"] == "mcp"
    assert metadata["version"] == "0.1.2"
    assert metadata["minWorkbuddyVersion"] == "4.24.0"
    assert len(metadata["examples_zh"]) >= 2
    assert len(metadata["examples_en"]) >= 2
    assert "auth_mode" not in metadata

    assert set(configuration) == {"mcpServers"}
    assert len(configuration["mcpServers"]) == 1
    server = configuration["mcpServers"]["m365"]
    assert server == {
        "type": "streamableHttp",
        "url": "${M365_MCP_URL}",
        "timeout": 30000,
    }
    assert not (CONNECTOR / "token-schema.json").exists()
    assert (CONNECTOR / "icon.svg").read_text("utf-8").lstrip().startswith("<svg")


def test_connector_skills_expose_only_their_scoped_mail_tools() -> None:
    expected = {
        "outlook-mail": {"mail_search", "mail_get"},
        "outlook-mail-compose": {"mail_create_draft", "mail_prepare_attachment_push", "mail_add_draft_attachment", "mail_list_attachments", "mail_send_draft", "Bash"},
        "outlook-attachments": {
            "mail_list_attachments",
            "mail_read_attachment",
            "mail_download_attachment",
        },
        "outlook-mail-management": {
            "mail_mark_read",
            "mail_archive",
            "mail_move",
            "mail_set_category",
        },
    }

    skills_root = CONNECTOR / "skills"
    assert {path.name for path in skills_root.iterdir() if path.is_dir()} == set(expected)
    for name, tools in expected.items():
        frontmatter = _frontmatter(skills_root / name / "SKILL.md")
        assert frontmatter["name"] == name
        assert frontmatter["version"] == "0.1.2"
        assert frontmatter["description_zh"]
        assert frontmatter["description_en"]
        assert {tool.strip() for tool in frontmatter["allowed-tools"].split(",")} == tools


def test_attachment_skill_defines_one_contextual_size_field() -> None:
    text = (CONNECTOR / "skills" / "outlook-attachments" / "SKILL.md").read_text(
        "utf-8"
    )

    assert "reported_size" not in text
    assert "Graph provider metadata only" in text
    assert "actual decoded byte length and is authoritative" in text


def test_compose_skill_bounds_automation_authorization() -> None:
    text = (CONNECTOR / "skills" / "outlook-mail-compose" / "SKILL.md").read_text(
        "utf-8"
    )
    normalized = " ".join(text.split())

    assert "### Interactive" in text
    assert "### Bounded automation" in text
    assert "allowed To, Cc, and Bcc recipients or domains" in text
    assert "maximum messages per run and per day" in text
    assert "an expiry or review date" in text
    assert "falls outside it" in text
    assert "can never create or expand an automation authorization" in normalized
    assert "Never automatically retry an ambiguous send failure" in text
