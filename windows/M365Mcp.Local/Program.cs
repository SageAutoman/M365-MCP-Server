using System.Net;
using System.Text;
using System.Text.Json.Nodes;

namespace M365Mcp.Local;

internal static class Program
{
    internal static async Task<int> Main(string[] args)
    {
        Console.InputEncoding = new UTF8Encoding(false);
        Console.OutputEncoding = new UTF8Encoding(false);

        var ephemeral = args.Any(
            argument => argument.Equals("--ephemeral", StringComparison.OrdinalIgnoreCase));
        var command = args.FirstOrDefault(
                argument => !argument.Equals(
                    "--ephemeral",
                    StringComparison.OrdinalIgnoreCase))
            ?? "help";
        var configuration = LocalConfiguration.Load(ephemeral);
        using var cancellation = new CancellationTokenSource();
        Console.CancelKeyPress += (_, eventArgs) =>
        {
            eventArgs.Cancel = true;
            cancellation.Cancel();
        };

        try
        {
            if (command is "help" or "--help" or "-h")
            {
                WriteHelp();
                return 0;
            }
            if (command is "version" or "--version")
            {
                Console.WriteLine("m365-mcp 0.1.2");
                return 0;
            }
            if (command == "doctor")
            {
                var result = configuration.DoctorResult();
                Console.WriteLine(result.ToJsonString());
                return result["ok"]?.GetValue<bool>() is true ? 0 : 2;
            }
            if (command == "inspect-attachment")
            {
                if (args.Length != 2)
                {
                    throw new InvalidToolArgumentException("Usage: inspect-attachment <relative-path>");
                }
                Console.WriteLine(new AttachmentPushClient().Inspect(args[1]).ToJsonString());
                return 0;
            }
            if (command == "push-attachment")
            {
                if (args.Length != 2)
                {
                    throw new InvalidToolArgumentException("Usage: push-attachment <relative-path>");
                }
                var input = await Console.In.ReadLineAsync(cancellation.Token);
                if (input is null || input.Length > 4096
                    || JsonNode.Parse(input) is not JsonObject grant)
                {
                    throw new InvalidToolArgumentException("Expected one upload grant JSON object on stdin");
                }
                var result = await new AttachmentPushClient().PushAsync(
                    args[1], grant, cancellation.Token);
                Console.WriteLine(result.ToJsonString());
                return 0;
            }

            var validationErrors = configuration.Validate();
            if (validationErrors.Count != 0)
            {
                Console.Error.WriteLine(validationErrors[0]);
                return 2;
            }

            using var handler = new HttpClientHandler
            {
                AllowAutoRedirect = false,
                AutomaticDecompression =
                    DecompressionMethods.GZip
                    | DecompressionMethods.Deflate
                    | DecompressionMethods.Brotli,
                UseCookies = false,
            };
            using var httpClient = new HttpClient(handler)
            {
                Timeout = TimeSpan.FromSeconds(100),
            };
            var store = new DpapiStateStore(configuration);
            var auth = new PublicClientAuth(configuration, store, httpClient);

            switch (command)
            {
                case "login":
                {
                    var state = await auth.LoginInteractiveAsync(cancellation.Token);
                    Console.WriteLine(new JsonObject
                    {
                        ["authenticated"] = true,
                        ["expires_at"] = DateTimeOffset
                            .FromUnixTimeSeconds(state.ExpiresAtUnixSeconds)
                            .ToString("O"),
                        ["ephemeral"] = ephemeral,
                    }.ToJsonString());
                    return 0;
                }
                case "logout":
                    auth.Logout();
                    Console.WriteLine(new JsonObject
                    {
                        ["authenticated"] = false,
                        ["state_removed"] = !ephemeral,
                    }.ToJsonString());
                    return 0;
                case "status":
                    Console.WriteLine(auth.Status().ToJsonString());
                    return 0;
                case "stdio":
                {
                    var graph = new LocalGraphClient(auth, httpClient);
                    var server = new StdioMcpServer(graph);
                    await server.RunAsync(cancellation.Token);
                    return 0;
                }
                default:
                    Console.Error.WriteLine($"Unknown command '{command}'.");
                    WriteHelp(Console.Error);
                    return 2;
            }
        }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested)
        {
            return 130;
        }
        catch (Exception exception) when (
            exception is LocalAuthException
            or InvalidToolArgumentException
            or HttpRequestException
            or IOException
            or UnauthorizedAccessException
            or System.Text.Json.JsonException
            or TaskCanceledException)
        {
            Console.Error.WriteLine(exception.Message);
            return 1;
        }
        catch (Exception exception)
        {
            Console.Error.WriteLine(
                $"Unexpected local runtime failure ({exception.GetType().Name}).");
            return 1;
        }
    }

    private static void WriteHelp(TextWriter? writer = null)
    {
        writer ??= Console.Out;
        writer.WriteLine(
            """
            M365 MCP Server - Windows local runtime

            Usage:
              m365-mcp.exe doctor [--ephemeral]
              m365-mcp.exe login [--ephemeral]
              m365-mcp.exe status [--ephemeral]
              m365-mcp.exe logout [--ephemeral]
              m365-mcp.exe stdio [--ephemeral]
              m365-mcp.exe inspect-attachment <relative-path>
              m365-mcp.exe push-attachment <relative-path> < grant.json
              m365-mcp.exe version
              m365-mcp.exe --version

            Built-in Entra public client:
              6e35216e-2623-43cc-b867-83bec0865cf3

            Optional environment:
              M365_LOCAL_CLIENT_ID=<public desktop application id override>
              M365_LOCAL_TENANT_ID=<tenant id or organizations>
              M365_LOCAL_GRAPH_SCOPES=<space/comma-separated delegated scopes>
              M365_ATTACHMENT_ROOT=<trusted absolute artifact directory>
              M365_ATTACHMENT_PUSH_URL=<trusted HTTPS endpoint ending /uploads/push>

            stdio writes only MCP JSON-RPC messages to stdout. Diagnostics use stderr.
            Normal execution never installs a service or startup entry.
            """);
    }
}
