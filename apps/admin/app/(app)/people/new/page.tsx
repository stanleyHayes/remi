import Link from "next/link";
import { PageHeader } from "@/components/ui";
import { PersonForm } from "@/components/people/PersonForm";

export default function NewPersonPage(){return <div><Link href="/people" className="member-back-link"><span aria-hidden="true">←</span> Back to people</Link><PageHeader title="Add person" subtitle="Create a respectful, usable member record with only the detail ministry work needs."/><PersonForm mode="create"/></div>}
