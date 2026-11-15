import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useIdentity } from "@infrapro/api-client/react";
import { Button } from "@infrapro/ui/components/button";
export function DisplayLaunch() {
  const { client, phase } = useIdentity();
  const [input] = useState(() => {
    const search = new URLSearchParams(location.search);
    const current = Object.fromEntries(search);
    if (current.instance && current.challenge) {
      return current;
    }
    try {
      return JSON.parse(
        sessionStorage.getItem("display_launch") ?? "{}",
      ) as Record<string, string>;
    } catch {
      return {};
    }
  });
  useEffect(() => {
    if (input.instance && input.challenge) {
      sessionStorage.setItem("display_launch", JSON.stringify(input));
      if (location.search) history.replaceState(null, "", location.pathname);
    }
  }, [input]);
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);

  async function launch() {
    setBusy(true);
    setError("");
    try {
      if (
        !/^[0-9a-f-]{36}$/.test(input.instance ?? "") ||
        !/^\w{64}$/.test(input.challenge ?? "")
      )
        throw Error("启动请求无效，请从大屏入口重新进入。");
      const target = new URL(input.audience ?? "");
      const local =
        target.protocol === "http:" &&
        ["127.0.0.1", "localhost", "[::1]"].includes(target.hostname) &&
        target.port !== "" &&
        target.pathname === "/" &&
        !target.username &&
        !target.search &&
        !target.hash;
      const action =
        input.purpose === "upload"
          ? "tool-authorize"
          : input.config
            ? "preview"
            : "launch";
      if (action === "tool-authorize" && !local)
        throw Error("工具仅支持本机配对。");
      const scope = await client.displayLocate(input.instance!);
      const result = await client.displayLaunch(
        { tenantId: scope.tenant_id, projectId: scope.project_id },
        input.instance!,
        action,
        {
          challenge: input.challenge,
          audience: target.origin,
          config_id: input.config || undefined,
        },
      );
      if (target.origin !== result.runtime_origin && !local)
        throw Error("运行来源不匹配。");
      target.pathname =
        target.origin === result.runtime_origin
          ? `/display/${input.instance}`
          : "/pair";
      target.hash = new URLSearchParams({
        code: result.code,
        ...(input.state ? { state: input.state } : {}),
      }).toString();
      sessionStorage.removeItem("display_launch");
      location.replace(target.href);
    } catch (e) {
      setError(e instanceof Error ? e.message : "启动失败");
      setBusy(false);
    }
  }
  return (
    <main className="mx-auto mt-20 max-w-xl space-y-5 rounded-xl border bg-card p-8">
      <p className="text-sm text-muted-foreground">InfraPro · 项目大屏</p>
      <h1 className="text-2xl font-semibold">
        {input.purpose === "upload"
          ? "授权本机工具上传"
          : input.config
            ? "进入真实草稿预览"
            : "打开项目大屏"}
      </h1>
      <p className="text-sm text-muted-foreground">
        {input.purpose === "upload"
          ? "本次授权只用于此实例的程序上传，10 分钟后失效。"
          : "将使用您的当前项目资格及大屏读取范围。"}
      </p>
      {phase !== "authenticated" ? (
        <Link to="/login" search={{ returnTo: "/display-launch" }}>
          登录后继续
        </Link>
      ) : (
        <Button disabled={busy} onClick={() => void launch()}>
          {busy ? "正在核验…" : "继续"}
        </Button>
      )}
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
    </main>
  );
}
