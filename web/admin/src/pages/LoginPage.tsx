import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button, ErrorBox, Field, Input, LoginLayout } from "ui";
import { api } from "../api";

export function LoginPage() {
  const nav = useNavigate();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: () => api.post("/auth/login", { email, password }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["me"] });
      nav("/clients");
    },
  });
  return (
    <LoginLayout title="Finenumbers SMS Service" subtitle="Админ панель">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          login.mutate();
        }}
      >
        <Field label="Эл. почта">
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label="Пароль">
          <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </Field>
        {login.isError ? <ErrorBox error={login.error} /> : null}
        <Button className="mt-3 w-full" type="submit" disabled={login.isPending}>
          Войти
        </Button>
      </form>
    </LoginLayout>
  );
}
