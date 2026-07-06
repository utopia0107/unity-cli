using System.Collections.Generic;
using System.Linq;
using System.Text.RegularExpressions;

namespace UnityCliConnector.Tools
{
    /// <summary>
    /// Parses csc console output into structured diagnostics and remaps line
    /// numbers from the generated wrapper source to the user's code.
    /// </summary>
    internal static class CscOutputParser
    {
        internal class Diagnostic
        {
            public int Line;     // 1-based line in the generated source
            public int Column;
            public string Severity; // "error" or "warning"
            public string Code;     // e.g. CS1002
            public string Message;
        }

        static readonly Regex DiagnosticRegex = new Regex(
            @"\((\d+),(\d+)\):\s*(error|warning)\s+(\w+):\s*(.+)", RegexOptions.Compiled);

        internal static List<Diagnostic> Parse(string output)
        {
            var result = new List<Diagnostic>();
            if (string.IsNullOrEmpty(output)) return result;

            foreach (var raw in output.Split('\n'))
            {
                var m = DiagnosticRegex.Match(raw.Trim());
                if (!m.Success) continue;
                result.Add(new Diagnostic
                {
                    Line = int.Parse(m.Groups[1].Value),
                    Column = int.Parse(m.Groups[2].Value),
                    Severity = m.Groups[3].Value,
                    Code = m.Groups[4].Value,
                    Message = m.Groups[5].Value.Trim(),
                });
            }
            return result;
        }

        /// <summary>
        /// Converts error diagnostics to user-facing entries. Line numbers are
        /// shifted by the generated header size so they point into the user's
        /// code; diagnostics outside the user snippet get line = null.
        /// </summary>
        internal static List<object> ToUserErrors(List<Diagnostic> diagnostics, int headerLines)
        {
            return diagnostics
                .Where(d => d.Severity == "error")
                .Select(d =>
                {
                    int userLine = d.Line - headerLines;
                    return (object)new
                    {
                        line = userLine >= 1 ? (int?)userLine : null,
                        column = d.Column,
                        code = d.Code,
                        message = d.Message,
                    };
                })
                .ToList();
        }
    }
}
