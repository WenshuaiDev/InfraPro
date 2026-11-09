import { Link } from '@tanstack/react-router';
import { ArrowLeft, LifeBuoy } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import { AuthCard } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';

export function ForgotPasswordPage() {
  const { help } = useIdentity();
  return <AuthCard title="找回账号密码" description="请联系部署维护人员，提供用户名并配合核验本人身份。" footer={<Button asChild variant="link" className="mx-auto"><Link to="/login"><ArrowLeft className="size-4" />返回登录</Link></Button>}>
    <div className="space-y-4 text-sm"><div className="rounded-md border bg-muted/40 p-4"><div className="mb-2 flex items-center gap-2 font-medium"><LifeBuoy className="size-4" />维护联系方式</div><p className="break-words">{help?.support_contact}</p></div><p className="leading-relaxed text-muted-foreground">核验通过后，维护人员会交付一次性重置链接。系统不会自动发送邮件或短信；请勿向他人提供当前密码。</p></div>
  </AuthCard>;
}
