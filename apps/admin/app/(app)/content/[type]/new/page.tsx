"use client";

import { useParams } from "next/navigation";
import ContentForm from "@/components/ContentForm";
import { getContentType } from "@/lib/content";
import { EmptyState } from "@/components/ui";

export default function NewContentPage() {
  const params = useParams<{ type: string }>();
  const def = getContentType(params.type);
  if (!def || !def.canCreate) {
    return <EmptyState title="Cannot create this content type" hint="This type is managed elsewhere." />;
  }
  return <ContentForm def={def} />;
}
