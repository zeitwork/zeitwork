<script setup lang="ts">
import {
  SearchIcon,
  SettingsIcon,
  ChevronRightIcon,
  ChevronDownIcon,
  ChevronsUpDownIcon,
  CircleCheckIcon,
  BoxIcon,
  GitBranchIcon,
  GitCommitHorizontalIcon,
  LoaderIcon,
  EllipsisIcon,
  ListIcon,
  LayoutGridIcon,
} from "lucide-vue-next";

definePageMeta({ layout: "auth" });
</script>

<template>
  <!-- base-0: page shell (full viewport) -->
  <div class="base-0 min-h-screen">

    <!-- Top nav — sits directly on base-0 -->
    <div class="flex items-center justify-between p-3">
      <div class="flex items-center gap-2">
        <div class="text-primary flex size-9 items-center justify-center">
          <svg class="size-[18px]" viewBox="0 0 18 18" fill="none">
            <path d="M0 0h18v9H9z" fill="currentColor" opacity="0.8" />
            <path d="M18 18H0V9h9z" fill="currentColor" opacity="0.8" />
          </svg>
        </div>
        <ChevronRightIcon class="text-tertiary size-4" />
        <div class="flex items-center gap-2 rounded-md px-2 py-1">
          <div class="bg-surface-1 size-6 overflow-hidden rounded-md ring-1 ring-edge">
            <div class="bg-surface-2 size-full"></div>
          </div>
          <span class="text-primary text-copy-sm font-medium">Dokedu</span>
          <ChevronsUpDownIcon class="text-tertiary size-4" />
        </div>
        <ChevronRightIcon class="text-tertiary size-4" />
        <div class="flex items-center gap-2 rounded-md px-2 py-1">
          <span class="text-primary text-copy-sm font-medium">Dokedu Backend</span>
          <ChevronsUpDownIcon class="text-tertiary size-4" />
        </div>
      </div>
      <div class="bg-surface-2 size-8 rounded-full"></div>
    </div>

    <!-- base-1: main content frame (inset from shell edges) -->
    <div class="flex flex-1 flex-col px-1 pb-1">
      <div class="base-1 flex flex-1 flex-col overflow-hidden rounded-lg ring-1 ring-edge">

        <!-- Tab bar — on base-1 -->
        <div class="flex items-center gap-2 p-2">
          <div class="text-secondary text-copy-sm flex items-center gap-2 rounded-md px-2 py-1.5">
            <LayoutGridIcon class="size-4" />
            <span>Overview</span>
          </div>
          <div class="bg-surface-2 text-primary text-copy-sm flex items-center gap-2 rounded-md px-2 py-1.5 font-medium">
            <BoxIcon class="size-4" />
            <span>Deployments</span>
          </div>
          <div class="text-secondary text-copy-sm flex items-center gap-2 rounded-md px-2 py-1.5">
            <ListIcon class="size-4" />
            <span>Logs</span>
          </div>
          <div class="text-secondary text-copy-sm flex items-center gap-2 rounded-md px-2 py-1.5">
            <SettingsIcon class="size-4" />
            <span>Settings</span>
          </div>
        </div>

        <!-- base-2: content card -->
        <div class="base-2 flex flex-1 flex-col overflow-hidden rounded-lg ring-1 ring-edge">

          <!-- Content header -->
          <div class="flex items-center justify-between p-2">
            <div class="flex items-center gap-2">
              <span class="text-primary text-copy-sm px-2 py-1.5 font-medium">Deployments</span>
              <span class="text-tertiary text-copy-sm">Automatically created for pushes to</span>
              <div class="flex items-center gap-1">
                <div class="bg-surface-1 size-3.5 rounded-full"></div>
                <span class="text-primary text-copy-xs font-mono">dokedu/dokedu</span>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <!-- Filter: branches — surface-1 on base-2 -->
              <div class="bg-surface-1 flex h-8 w-[230px] items-center justify-between rounded-md ring-1 ring-edge px-2">
                <div class="flex items-center gap-2">
                  <SearchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm">All Branches...</span>
                </div>
                <ChevronDownIcon class="text-tertiary size-4" />
              </div>
              <!-- Filter: environments — surface-1 on base-2 -->
              <div class="bg-surface-1 flex h-8 w-[230px] items-center justify-between rounded-md ring-1 ring-edge px-2">
                <span class="text-primary text-copy-sm">All Environments</span>
                <ChevronDownIcon class="text-tertiary size-4" />
              </div>
            </div>
          </div>

          <!-- Deployment list — inner rounded frame -->
          <div class="flex-1 overflow-hidden rounded-lg ring-1 ring-edge shadow-sm">
            <!-- Row 1: Building -->
            <div class="border-edge flex items-center border-b p-[18px] transition-colors hover:bg-surface-1">
              <div class="flex w-[200px] shrink-0 flex-col gap-1.5">
                <span class="text-primary text-copy-sm font-medium font-mono">EjnklDBk2</span>
                <span class="text-tertiary text-copy-sm">Preview</span>
              </div>
              <div class="flex w-[120px] shrink-0 flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <div class="size-2.5 rounded-full bg-orange-400"></div>
                  <span class="text-tertiary text-copy-sm">Building</span>
                </div>
                <div class="flex items-center gap-1.5">
                  <LoaderIcon class="text-tertiary size-2.5" />
                  <span class="text-tertiary text-copy-sm">12s</span>
                </div>
              </div>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <div class="flex items-center gap-1">
                  <GitBranchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm">dev</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex shrink-0 items-center gap-1">
                    <GitCommitHorizontalIcon class="text-tertiary size-4" />
                    <span class="text-tertiary text-copy-sm font-mono">5315ffa</span>
                  </div>
                  <span class="text-primary text-copy-sm truncate">Add validation for document fields as they need to be validated</span>
                </div>
              </div>
              <div class="flex w-[300px] shrink-0 items-center justify-end gap-6">
                <div class="flex items-center gap-1.5">
                  <span class="text-tertiary text-copy-sm">25s ago by</span>
                  <div class="bg-surface-2 size-6 rounded-full"></div>
                  <span class="text-tertiary text-copy-sm">aaronmahlke</span>
                </div>
                <EllipsisIcon class="text-tertiary size-5" />
              </div>
            </div>

            <!-- Row 2: Ready + Current -->
            <div class="border-edge flex items-center border-b p-[18px] transition-colors hover:bg-surface-1">
              <div class="flex w-[200px] shrink-0 items-center gap-4">
                <div class="flex flex-col gap-1.5">
                  <span class="text-primary text-copy-sm font-medium font-mono">EjnklDBk2</span>
                  <span class="text-tertiary text-copy-sm">Production</span>
                </div>
                <div class="bg-accent-subtle text-accent flex items-center gap-1 rounded-full py-1 pl-1 pr-1.5">
                  <CircleCheckIcon class="size-3.5" />
                  <span class="text-copy-xs">Current</span>
                </div>
              </div>
              <div class="flex w-[120px] shrink-0 flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <div class="size-2.5 rounded-full bg-green-500"></div>
                  <span class="text-tertiary text-copy-sm">Ready</span>
                </div>
                <span class="text-tertiary text-copy-sm">34s (5m ago)</span>
              </div>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <div class="flex items-center gap-1">
                  <GitBranchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm font-mono">main</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex shrink-0 items-center gap-1">
                    <GitCommitHorizontalIcon class="text-tertiary size-4" />
                    <span class="text-tertiary text-copy-sm font-mono">5315ffa</span>
                  </div>
                  <span class="text-primary text-copy-sm truncate">Add export functionality for entries with PDF generation</span>
                </div>
              </div>
              <div class="flex w-[300px] shrink-0 items-center justify-end gap-6">
                <div class="flex items-center gap-1.5">
                  <span class="text-tertiary text-copy-sm">5d ago by</span>
                  <div class="bg-surface-2 size-6 rounded-full"></div>
                  <span class="text-tertiary text-copy-sm">aaronmahlke</span>
                </div>
                <EllipsisIcon class="text-tertiary size-5" />
              </div>
            </div>

            <!-- Row 3: Ready -->
            <div class="border-edge flex items-center border-b p-[18px] transition-colors hover:bg-surface-1">
              <div class="flex w-[200px] shrink-0 flex-col gap-1.5">
                <span class="text-primary text-copy-sm font-medium font-mono">EjnklDBk2</span>
                <span class="text-tertiary text-copy-sm">Preview</span>
              </div>
              <div class="flex w-[120px] shrink-0 flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <div class="size-2.5 rounded-full bg-green-500"></div>
                  <span class="text-tertiary text-copy-sm">Ready</span>
                </div>
                <span class="text-tertiary text-copy-sm">34s (5m ago)</span>
              </div>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <div class="flex items-center gap-1">
                  <GitBranchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm">dev</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex shrink-0 items-center gap-1">
                    <GitCommitHorizontalIcon class="text-tertiary size-4" />
                    <span class="text-tertiary text-copy-sm font-mono">5315ffa</span>
                  </div>
                  <span class="text-primary text-copy-sm truncate">Refactor reports page to implement sorting</span>
                </div>
              </div>
              <div class="flex w-[300px] shrink-0 items-center justify-end gap-6">
                <div class="flex items-center gap-1.5">
                  <span class="text-tertiary text-copy-sm">25 ago by</span>
                  <div class="bg-surface-2 size-6 rounded-full"></div>
                  <span class="text-tertiary text-copy-sm">aaronmahlke</span>
                </div>
                <EllipsisIcon class="text-tertiary size-5" />
              </div>
            </div>

            <!-- Row 4: Ready -->
            <div class="border-edge flex items-center border-b p-[18px] transition-colors hover:bg-surface-1">
              <div class="flex w-[200px] shrink-0 flex-col gap-1.5">
                <span class="text-primary text-copy-sm font-medium font-mono">EjnklDBk2</span>
                <span class="text-tertiary text-copy-sm">Production</span>
              </div>
              <div class="flex w-[120px] shrink-0 flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <div class="size-2.5 rounded-full bg-green-500"></div>
                  <span class="text-tertiary text-copy-sm">Ready</span>
                </div>
                <span class="text-tertiary text-copy-sm">34s (5m ago)</span>
              </div>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <div class="flex items-center gap-1">
                  <GitBranchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm font-mono">main</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex shrink-0 items-center gap-1">
                    <GitCommitHorizontalIcon class="text-tertiary size-4" />
                    <span class="text-tertiary text-copy-sm font-mono">5315ffa</span>
                  </div>
                  <span class="text-primary text-copy-sm truncate">Implement competence history and level adjustment features</span>
                </div>
              </div>
              <div class="flex w-[300px] shrink-0 items-center justify-end gap-6">
                <div class="flex items-center gap-1.5">
                  <span class="text-tertiary text-copy-sm">25 ago by</span>
                  <div class="bg-surface-2 size-6 rounded-full"></div>
                  <span class="text-tertiary text-copy-sm">aaronmahlke</span>
                </div>
                <EllipsisIcon class="text-tertiary size-5" />
              </div>
            </div>

            <!-- Row 5: In Progress -->
            <div class="flex items-center p-[18px] transition-colors hover:bg-surface-1">
              <div class="flex w-[200px] shrink-0 flex-col gap-1.5">
                <span class="text-primary text-copy-sm font-medium font-mono">EjnklDBk2</span>
                <span class="text-tertiary text-copy-sm">Production</span>
              </div>
              <div class="flex w-[120px] shrink-0 flex-col gap-1.5">
                <div class="flex items-center gap-1.5">
                  <div class="size-2.5 rounded-full bg-yellow-500"></div>
                  <span class="text-tertiary text-copy-sm">In Progress</span>
                </div>
                <span class="text-tertiary text-copy-sm">34s (5m ago)</span>
              </div>
              <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                <div class="flex items-center gap-1">
                  <GitBranchIcon class="text-tertiary size-4" />
                  <span class="text-tertiary text-copy-sm">dev</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex shrink-0 items-center gap-1">
                    <GitCommitHorizontalIcon class="text-tertiary size-4" />
                    <span class="text-tertiary text-copy-sm font-mono">5315ffa</span>
                  </div>
                  <span class="text-primary text-copy-sm truncate">Enhance error handling in user authentication</span>
                </div>
              </div>
              <div class="flex w-[300px] shrink-0 items-center justify-end gap-6">
                <div class="flex items-center gap-1.5">
                  <span class="text-tertiary text-copy-sm">5 mins ago by</span>
                  <div class="bg-surface-2 size-6 rounded-full"></div>
                  <span class="text-tertiary text-copy-sm">johndoe</span>
                </div>
                <EllipsisIcon class="text-tertiary size-5" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
