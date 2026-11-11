import { useState } from 'react';
import { FileJsonIcon, WrapTextIcon } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { CodeBlock, CodeBlockActions, CodeBlockContent, CodeBlockCopyButton, CodeBlockHeader, CodeBlockLine, CodeBlockTitle } from '@infrapro/ui/components/code-block';
import { useHighlightedCode } from '@infrapro/ui/hooks/use-highlighted-code';

export function AuditRawContent({ value }: { value: string }) {
  const [wrap, setWrap] = useState(false);
  const lines = useHighlightedCode(value, 'json');

  return <CodeBlock size="sm">
    <CodeBlockHeader className="border-b bg-muted/40 pb-2.5">
      <CodeBlockTitle><FileJsonIcon />审计详情.json</CodeBlockTitle>
      <CodeBlockActions>
        <Button type="button" variant="ghost" size="icon" aria-label="自动换行" title="自动换行" aria-pressed={wrap} className="aria-pressed:bg-accent" onClick={() => setWrap(!wrap)}><WrapTextIcon /></Button>
        <CodeBlockCopyButton value={value} />
      </CodeBlockActions>
    </CodeBlockHeader>
    <CodeBlockContent aria-label="原始审计内容" lineNumbers wrap={wrap} className="max-h-[min(32rem,55dvh)] overscroll-contain leading-5">
      {lines.map((line, index) => <CodeBlockLine key={index}>{line}</CodeBlockLine>)}
    </CodeBlockContent>
  </CodeBlock>;
}
