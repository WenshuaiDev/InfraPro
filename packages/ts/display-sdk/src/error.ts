export class DisplayError extends Error {
  readonly code: string;
  readonly issues: readonly { path: string; code: string; message: string }[];
  constructor(
    code: string,
    message = code,
    issues: readonly { path: string; code: string; message: string }[] = [],
  ) {
    super(message);
    this.name = "DisplayError";
    this.code = code;
    this.issues = issues;
  }
  static from(value: unknown): DisplayError {
    if (value instanceof DisplayError) return value;
    if (
      value &&
      typeof value === "object" &&
      "code" in value &&
      typeof value.code === "string"
    ) {
      const message =
        "message" in value && typeof value.message === "string"
          ? value.message
          : value.code;
      const issues = "issues" in value && Array.isArray(value.issues)
        ? value.issues.filter((issue): issue is {path:string;code:string;message:string} =>
          !!issue && typeof issue === "object" && typeof issue.path === "string" && typeof issue.code === "string" && typeof issue.message === "string")
        : [];
      return new DisplayError(value.code, message, issues);
    }
    return new DisplayError("dependency_unavailable", "显示服务暂不可用。");
  }
}
