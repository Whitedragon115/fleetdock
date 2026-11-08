"use client";

import { usePathname } from "next/navigation";

import { DataBrowserSkeleton } from "@/components/data-browser/skeleton";
import { PageSkeleton } from "@/components/ui";

// Shown while a page's code loads on navigation; pages show their own
// skeletons while their data loads.
//
// This boundary is the one used when arriving from another section (e.g.
// Backups → Data Browser), so it picks the skeleton for the destination: the
// Data Browser is a full-bleed workspace (see the <main> class in layout.tsx)
// and gets a skeleton of its own layout.
export default function DashboardLoading() {
  const pathname = usePathname();
  return pathname === "/data" ? <DataBrowserSkeleton /> : <PageSkeleton />;
}
