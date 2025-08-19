<template>
  <div class="bg-neutral-strong flex h-screen flex-row">
    <div class="flex flex-1 flex-col p-2">
      <div class="border-neutral bg-surface-weak flex-1 overflow-auto rounded-lg border">
        <slot></slot>
      </div>
    </div>
  </div>
</template>

<style lang="postcss">
html,
body {
  overflow: hidden;
  @apply bg-neutral-50;
}
</style>
