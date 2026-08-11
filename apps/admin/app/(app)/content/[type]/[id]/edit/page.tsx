"use client";

import { useParams } from "next/navigation";
import ContentForm from "@/components/ContentForm";
import { getContentType } from "@/lib/content";
import { EmptyState } from "@/components/ui";

export default function EditContentPage() {
  const params = useParams<{ type: string; id: string }>();
  const def = getContentType(params.type);
  if (!def) return <EmptyState title="Unknown content type" />;
  return <ContentForm def={def} docId={params.id} />;
}
