<script setup lang="ts">
import {
  CheckCircleIcon,
  ExclamationTriangleIcon,
  GlobeAltIcon,
} from "@heroicons/vue/16/solid";
import { GitMergeIcon, LoaderIcon } from "lucide-vue-next";
import { useIntervalFn } from "@vueuse/core";

type Domain = {
  id: string;
  name: string;
  target?: string;
  verifiedAt?: string | null;
  txtVerificationRequired?: boolean;
};

type Props = {
  domain: Domain;
  projectSlug: string;
};

type Emits = {
  setup: [];
  edit: [];
  verified: [];
};

const { domain, projectSlug } = defineProps<Props>();
const emit = defineEmits<Emits>();

const isVerified = ref(!!domain.verifiedAt);
const isProduction = computed(() => !domain.target || domain.target === "production");

const shouldPoll = computed(() => !isVerified.value);

const { data: verifyData, refresh } = await useFetch(
  `/api/projects/${projectSlug}/domains/${domain.id}/verify`,
  { immediate: shouldPoll.value },
);

const { pause, resume } = useIntervalFn(
  () => {
    if (shouldPoll.value) refresh();
  },
  5000,
  { immediate: false },
);

onMounted(() => {
  if (shouldPoll.value) resume();
});

onUnmounted(() => {
  pause();
});

watch(
  () => verifyData.value?.verified,
  (verified) => {
    if (verified) {
      isVerified.value = true;
      pause();
      emit("verified");
    }
  },
);
</script>

<template>
  <div class="border-edge flex items-center gap-6 border-b p-4.5">

    <div class="flex w-[400px] items-center gap-3">
      <CheckCircleIcon v-if="isVerified" class="size-4 shrink-0 text-success" />
      <ExclamationTriangleIcon v-else class="size-4 shrink-0 text-warn" />
      <div class="flex flex-col gap-1.5">
        <span class="text-copy text-primary">{{ domain.name }}</span>
        <div class="flex items-center gap-1.5">
          <span
            v-if="isVerified"
            class="bg-surface-1 text-primary rounded px-1 py-0.5 text-xs font-medium"
          >
            Valid configuration
          </span>
          <template v-else>
            <span class="rounded bg-warn-subtle px-1 py-0.5 text-xs font-medium text-warn">
              Verification needed
            </span>
            <div class="text-secondary flex items-center gap-1.5">
              <LoaderIcon class="size-3.5 animate-spin" />
              <span class="text-xs font-medium">Verifying</span>
            </div>
          </template>
        </div>
      </div>
    </div>


    <div class="text-primary flex flex-1 items-center gap-2">
      <GlobeAltIcon v-if="isProduction" class="size-4" />
      <GitMergeIcon v-else class="size-4" />
      <span class="text-copy">{{ isProduction ? "Production" : "Development" }}</span>
    </div>


    <div class="flex w-[300px] items-center justify-end gap-2">
      <DButton v-if="!isVerified" size="sm" @click="emit('setup')">Setup</DButton>
      <DButton variant="secondary" size="sm" @click="emit('edit')">Edit</DButton>
    </div>
  </div>
</template>
