"use client";

import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import DatabaseOverviewPage from "@/components/databases/database-overview-page";

export default function DatabaseConsolePage() {
  const params = useParams();
  const databaseId = params.id as string;

  const { data: database, isLoading } = useQuery({
    queryKey: ["database", databaseId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/databases/${databaseId}`);
      if (!res.ok) throw new Error("Failed to fetch database");
      const data = await res.json();
      return data.data;
    },
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  return (
    <DatabaseOverviewPage
      database={database}
      accountId={params.account_id as string}
    />
  );
}
