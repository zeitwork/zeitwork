<template>
  <div class="bg-surface-2 flex h-screen flex-row">
    <div class="flex flex-1 flex-col p-2">
      <div class="border-edge bg-surface-1 flex-1 overflow-auto rounded-lg border">
        <slot></slot>
      </div>
    </div>
  </div>
</template>
