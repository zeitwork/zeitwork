<script setup lang="ts">
import { useIntervalFn } from "@vueuse/core";
import { CheckCircleIcon } from "@heroicons/vue/16/solid";
import { CopyIcon, LoaderIcon } from "lucide-vue-next";

type Domain = {
  id: string;
  name: string;
  target?: string;
  verifiedAt?: string | null;
};

type Props = {
  domain: Domain;
  projectSlug: string;
};

type Emits = {
  verified: [];
};

const open = defineModel<boolean>();
const { domain, projectSlug } = defineProps<Props>();
const emit = defineEmits<Emits>();

const config = useRuntimeConfig();

const isVerified = ref(!!domain.verifiedAt);

const { data: verifyData, refresh } = await useFetch(
  `/api/projects/${projectSlug}/domains/${domain.id}/verify`,
);

const { pause, resume } = useIntervalFn(
  () => {
    refresh();
  },
  5000,
  { immediate: false },
);

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

watch(open, (isOpen) => {
  if (isOpen && !isVerified.value) {
    resume();
  } else {
    pause();
  }
});

onUnmounted(() => {
  pause();
});

const dnsRecord = computed(() => {
  const dotCount = (domain.name.match(/\./g) || []).length;

  if (dotCount === 1) {
    return {
      type: "A",
      name: "@",
      value: config.public.edgeIp as string,
    };
  }

  const subdomain = domain.name.split(".").slice(0, -2).join(".");
  return {
    type: "CNAME",
    name: subdomain,
    value: config.public.edgeDomain as string,
  };
});

const txtName = computed(() => `zeitwork_verify_${domain.name}`);
const txtValue = computed(() => uuidToBase58(domain.id));

const copied = ref<string | null>(null);

async function copyToClipboard(text: string, field: string) {
  await navigator.clipboard.writeText(text);
  copied.value = field;
  setTimeout(() => {
    copied.value = null;
  }, 2000);
}
</script>

<template>
  <DDialog v-model="open" size="lg">
    <template #title>Setup domain</template>
    <template #content>
      <div class="flex flex-col gap-3">
        <!-- Status row -->
        <div class="flex items-center justify-between">
          <span
            v-if="isVerified"
            class="rounded bg-green-100 px-1.5 py-0.5 text-xs font-medium text-green-700"
          >
            Verified
          </span>
          <span v-else class="rounded bg-orange-100 px-1.5 py-0.5 text-xs font-medium text-orange-700">
            Setup needed
          </span>
          <div v-if="!isVerified" class="text-neutral-subtle flex items-center gap-1.5">
            <LoaderIcon class="size-4 animate-spin" />
            <span class="text-copy">Verifying</span>
          </div>
          <div v-else class="text-green-600 flex items-center gap-1.5">
            <CheckCircleIcon class="size-4" />
            <span class="text-copy">Verified</span>
          </div>
        </div>

        <!-- Description -->
        <p class="text-copy text-neutral-subtle leading-relaxed">
          Setup these DNS records at your provider to connect your domain to Zeitwork:
        </p>

        <!-- DNS records table -->
        <div class="border-neutral overflow-hidden rounded-md border shadow-xs">
          <!-- Header -->
          <div class="border-neutral flex border-b">
            <div class="text-neutral-subtle w-[100px] px-3 py-2 text-sm font-medium">Type</div>
            <div class="text-neutral-subtle w-[135px] px-3 py-2 text-sm font-medium">Name</div>
            <div class="text-neutral-subtle flex-1 px-3 py-2 text-sm font-medium">Value</div>
          </div>
          <!-- A/CNAME row -->
          <div class="border-neutral flex items-center border-b">
            <div class="text-neutral w-[100px] px-3 py-2 text-sm font-medium">
              {{ dnsRecord.type }}
            </div>
            <div class="flex w-[135px] items-center gap-2 px-3 py-2">
              <input
                type="text"
                readonly
                :value="dnsRecord.name"
                class="text-neutral w-full truncate bg-transparent text-sm font-medium outline-none"
              />
              <button
                class="text-neutral-subtle hover:text-neutral shrink-0 cursor-pointer rounded p-0.5 transition-colors"
                @click="copyToClipboard(dnsRecord.name, 'dns-name')"
              >
                <CopyIcon class="size-4" />
              </button>
            </div>
            <div class="flex flex-1 items-center gap-2 px-3 py-2">
              <input
                type="text"
                readonly
                :value="dnsRecord.value"
                class="text-neutral w-full truncate bg-transparent text-sm font-medium outline-none"
              />
              <button
                class="text-neutral-subtle hover:text-neutral shrink-0 cursor-pointer rounded p-0.5 transition-colors"
                @click="copyToClipboard(dnsRecord.value, 'dns-value')"
              >
                <CopyIcon class="size-4" />
              </button>
            </div>
          </div>
          <!-- TXT row -->
          <div class="flex items-center">
            <div class="text-neutral w-[100px] px-3 py-2 text-sm font-medium">TXT</div>
            <div class="flex w-[135px] items-center gap-2 px-3 py-2">
              <input
                type="text"
                readonly
                :value="txtName"
                class="text-neutral w-full truncate bg-transparent text-sm font-medium outline-none"
              />
              <button
                class="text-neutral-subtle hover:text-neutral shrink-0 cursor-pointer rounded p-0.5 transition-colors"
                @click="copyToClipboard(txtName, 'txt-name')"
              >
                <CopyIcon class="size-4" />
              </button>
            </div>
            <div class="flex flex-1 items-center gap-2 px-3 py-2">
              <input
                type="text"
                readonly
                :value="txtValue"
                class="text-neutral w-full truncate bg-transparent text-sm font-medium outline-none"
              />
              <button
                class="text-neutral-subtle hover:text-neutral shrink-0 cursor-pointer rounded p-0.5 transition-colors"
                @click="copyToClipboard(txtValue, 'txt-value')"
              >
                <CopyIcon class="size-4" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>
    <template #cancel>
      <DButton variant="secondary">Cancel</DButton>
    </template>
    <template #submit>
      <DButton @click="open = false">Done</DButton>
    </template>
  </DDialog>
</template>
