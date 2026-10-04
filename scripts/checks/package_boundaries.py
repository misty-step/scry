#!/usr/bin/env python3
"""Reject dependencies and raw policy strings that cross Scry boundaries."""

import re
import sys
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
BOUNDARY_PACKAGES = ("internal/store", "internal/learning")
FORBIDDEN_INTERNAL_IMPORTS = ("internal/semantic", "internal/generation", "internal/web")
RAW_GRADING_WRAPPERS = {"Grade", "EventAlgorithm"}


@dataclass(frozen=True)
class Token:
    kind: str
    value: str
    line: int


def go_tokens(source):
    """Return the small Go token subset needed by this check."""
    tokens = []
    index = 0
    line = 1
    length = len(source)
    while index < length:
        char = source[index]
        if char.isspace():
            line += char == "\n"
            index += 1
            continue
        if source.startswith("//", index):
            end = source.find("\n", index + 2)
            index = length if end < 0 else end
            continue
        if source.startswith("/*", index):
            end = source.find("*/", index + 2)
            if end < 0:
                raise ValueError(f"unterminated block comment at line {line}")
            line += source.count("\n", index, end + 2)
            index = end + 2
            continue
        if char in {'"', "'", "`"}:
            start_line = line
            quote = char
            end = index + 1
            if quote == "`":
                while end < length and source[end] != "`":
                    line += source[end] == "\n"
                    end += 1
            else:
                while end < length and source[end] != quote:
                    if source[end] == "\\":
                        end += 2
                        continue
                    line += source[end] == "\n"
                    end += 1
            if end >= length:
                raise ValueError(f"unterminated Go literal at line {start_line}")
            kind = "string" if quote != "'" else "rune"
            tokens.append(Token(kind, source[index + 1:end], start_line))
            index = end + 1
            continue
        match = re.match(r"[A-Za-z_][A-Za-z0-9_]*", source[index:])
        if match:
            value = match.group(0)
            tokens.append(Token("identifier", value, line))
            index += len(value)
            continue
        tokens.append(Token("punctuation", char, line))
        index += 1
    return tokens


def matching(tokens, start, opening, closing):
    depth = 0
    for index in range(start, len(tokens)):
        if tokens[index].value == opening:
            depth += 1
        elif tokens[index].value == closing:
            depth -= 1
            if depth == 0:
                return index
    raise ValueError(f"unmatched {opening!r} at line {tokens[start].line}")


def imports(tokens):
    result = []
    for index, token in enumerate(tokens):
        if token.value != "import" or index + 1 >= len(tokens):
            continue
        cursor = index + 1
        if tokens[cursor].value == "(":
            end = matching(tokens, cursor, "(", ")")
            result.extend(item.value for item in tokens[cursor + 1:end] if item.kind == "string")
            continue
        while cursor < len(tokens) and tokens[cursor].line == token.line:
            if tokens[cursor].kind == "string":
                result.append(tokens[cursor].value)
                break
            cursor += 1
    return result


def function_parameters(tokens):
    """Yield function name, declaration line, and outer parameter tokens."""
    index = 0
    while index < len(tokens):
        if tokens[index].value != "func":
            index += 1
            continue
        cursor = index + 1
        if cursor < len(tokens) and tokens[cursor].value == "(":
            cursor = matching(tokens, cursor, "(", ")") + 1
        if cursor >= len(tokens) or tokens[cursor].kind != "identifier":
            index += 1
            continue
        name = tokens[cursor].value
        line = tokens[cursor].line
        cursor += 1
        if cursor < len(tokens) and tokens[cursor].value == "[":
            cursor = matching(tokens, cursor, "[", "]") + 1
        if cursor >= len(tokens) or tokens[cursor].value != "(":
            index += 1
            continue
        end = matching(tokens, cursor, "(", ")")
        yield name, line, tokens[cursor + 1:end]
        index = end + 1


def has_raw_grading_string(parameters):
    depth = 0
    for index, token in enumerate(parameters[:-1]):
        if token.value in "([{":
            depth += 1
            continue
        if token.value in ")]}":
            depth -= 1
            continue
        if depth == 0 and token.value == "grading" and parameters[index + 1].value == "string":
            return True
    return False


def go_files(relative):
    return sorted((ROOT / relative).rglob("*.go"))


def relative(file):
    return file.relative_to(ROOT).as_posix()


def module_path():
    match = re.search(r"(?m)^module\s+(\S+)\s*$", (ROOT / "go.mod").read_text())
    if not match:
        raise ValueError("go.mod has no module declaration")
    return match.group(1)


def check():
    violations = []
    module = module_path()
    forbidden_imports = {
        "net/http",
        *(f"{module}/{path}" for path in FORBIDDEN_INTERNAL_IMPORTS),
    }
    for package in BOUNDARY_PACKAGES:
        for file in go_files(package):
            tokens = go_tokens(file.read_text())
            for imported in imports(tokens):
                if imported in forbidden_imports or any(
                    imported.startswith(path + "/") for path in forbidden_imports
                ):
                    violations.append(f"{relative(file)}: forbidden import {imported}")

    for file in go_files("internal/web"):
        tokens = go_tokens(file.read_text())
        for index in range(len(tokens) - 2):
            if [item.value for item in tokens[index:index + 3]] == ["template", ".", "HTML"]:
                violations.append(f"{relative(file)}:{tokens[index].line}: template.HTML is forbidden")

    for file in go_files("internal/learning"):
        tokens = go_tokens(file.read_text())
        for name, line, parameters in function_parameters(tokens):
            if name not in RAW_GRADING_WRAPPERS and has_raw_grading_string(parameters):
                violations.append(
                    f"{relative(file)}:{line}: {name} takes raw grading string"
                )
    return violations


def main():
    try:
        violations = check()
    except (OSError, ValueError) as error:
        sys.exit(f"package-boundaries: {error}")
    if violations:
        sys.exit("package-boundaries: FAIL\n" + "\n".join(f"- {item}" for item in violations))
    print("package-boundaries: PASS")


if __name__ == "__main__":
    main()
