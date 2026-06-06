import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { flushPromises, mount } from "@vue/test-utils";

import type { AdminGroup } from "@/types";
import GroupsView from "../GroupsView.vue";

const {
  createGroup,
  updateGroup,
  listGroups,
  getUsageSummary,
  getCapacitySummary,
  getModelsListCandidates,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  listGroups: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getModelsListCandidates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/api/admin", () => ({
  adminAPI: {
    groups: {
      create: createGroup,
      update: updateGroup,
      delete: vi.fn(),
      getAll: vi.fn(),
      list: listGroups,
      getUsageSummary,
      getCapacitySummary,
      getModelsListCandidates,
      updateSortOrder: vi.fn(),
    },
    accounts: {
      list: vi.fn(),
      getById: vi.fn(),
    },
  },
}));

vi.mock("@/stores/app", () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}));

vi.mock("@/stores/onboarding", () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  }),
}));

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  const translations: Record<string, string> = {
    "admin.groups.createGroup": "Create group",
    "admin.groups.form.name": "Name",
    "admin.groups.form.description": "Description",
    "admin.groups.form.platform": "Platform",
    "admin.groups.form.rateMultiplier": "Rate multiplier",
    "admin.groups.form.rpmLimit": "RPM limit",
    "admin.groups.form.rateLimit5h": "5-hour USD limit",
    "admin.groups.form.status": "Status",
    "admin.groups.subscription.type": "Billing type",
    "admin.groups.subscription.standard": "Standard",
    "admin.groups.subscription.subscription": "Subscription",
    "admin.groups.subscription.dailyLimit": "Daily limit",
    "admin.groups.subscription.weeklyLimit": "Weekly limit",
    "admin.groups.subscription.monthlyLimit": "Monthly limit",
    "admin.groups.subscription.noLimit": "No limit",
    "admin.groups.subscription.typeHint": "Billing type hint",
    "admin.groups.subscription.typeNotEditable": "Billing type is not editable.",
    "admin.groups.groupCreated": "Group created",
    "admin.groups.groupUpdated": "Group updated",
    "admin.groups.columns.name": "Name",
    "admin.groups.columns.platform": "Platform",
    "admin.groups.columns.billingType": "Billing type",
    "admin.groups.columns.rateMultiplier": "Rate multiplier",
    "admin.groups.columns.rateLimit5h": "5-hour limit",
    "admin.groups.columns.type": "Type",
    "admin.groups.columns.accounts": "Accounts",
    "admin.groups.columns.capacity": "Capacity",
    "admin.groups.columns.usage": "Usage",
    "admin.groups.columns.status": "Status",
    "admin.groups.columns.actions": "Actions",
    "admin.accounts.status.active": "Active",
    "admin.accounts.status.inactive": "Inactive",
    "common.edit": "Edit",
    "common.save": "Save",
    "common.create": "Create",
    "common.cancel": "Cancel",
    "common.refresh": "Refresh",
  };
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => translations[key] ?? key,
    }),
  };
});

const SelectStub = defineComponent({
  name: "SelectStub",
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: "",
    },
    options: {
      type: Array,
      default: () => [],
    },
    disabled: {
      type: Boolean,
      default: false,
    },
  },
  emits: ["update:modelValue", "change"],
  setup(props, { emit }) {
    return () =>
      h(
        "select",
        {
          disabled: props.disabled,
          value: props.modelValue ?? "",
          onChange: (event: Event) => {
            const value = (event.target as HTMLSelectElement).value;
            emit("update:modelValue", value);
            emit("change", value, null);
          },
        },
        (props.options as Array<{ value: string | number; label: string }>).map((option) =>
          h("option", { value: option.value }, option.label),
        ),
      );
  },
});

const DataTableStub = {
  props: ["columns", "data"],
  emits: ["sort"],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-rate_limit_5h" :value="row.rate_limit_5h" :row="row" />
        <div data-test="row-actions">
          <slot name="cell-actions" :value="row.actions" :row="row" />
        </div>
      </div>
      <slot v-if="!data.length" name="empty" />
    </div>
  `,
};

const BaseDialogStub = {
  props: ["show", "title"],
  template: '<section v-if="show"><h2>{{ title }}</h2><slot /><slot name="footer" /></section>',
};

const defaultGroups = (): AdminGroup[] => [
  {
    id: 1,
    name: "standard-group",
    description: "",
    platform: "anthropic",
    rate_multiplier: 1,
    rate_limit_5h: 25,
    is_exclusive: false,
    status: "active",
    subscription_type: "standard",
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: "2026-06-06T00:00:00Z",
    updated_at: "2026-06-06T00:00:00Z",
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: true,
    sort_order: 10,
  },
  {
    id: 2,
    name: "subscription-group",
    description: "",
    platform: "anthropic",
    rate_multiplier: 1,
    rate_limit_5h: 50,
    is_exclusive: true,
    status: "active",
    subscription_type: "subscription",
    daily_limit_usd: 100,
    weekly_limit_usd: 500,
    monthly_limit_usd: 1000,
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: "2026-06-06T00:00:00Z",
    updated_at: "2026-06-06T00:00:00Z",
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: true,
    sort_order: 20,
  },
];

const mountView = async () => {
  const wrapper = mount(GroupsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: "<div><slot /></div>" },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
        },
        DataTable: DataTableStub,
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Icon: true,
        PlatformIcon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: { template: "<div><slot /></div>" },
        Teleport: true,
      },
    },
  });
  await flushPromises();
  return wrapper;
};

describe("admin GroupsView subscription 5-hour limit", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    listGroups.mockReset();
    getUsageSummary.mockReset();
    getCapacitySummary.mockReset();
    getModelsListCandidates.mockReset();
    createGroup.mockReset();
    updateGroup.mockReset();
    showError.mockReset();
    showSuccess.mockReset();

    listGroups.mockResolvedValue({
      items: defaultGroups(),
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1,
    });
    getUsageSummary.mockResolvedValue([]);
    getCapacitySummary.mockResolvedValue([]);
    getModelsListCandidates.mockResolvedValue([]);
    createGroup.mockResolvedValue(defaultGroups()[1]);
    updateGroup.mockResolvedValue(defaultGroups()[1]);
  });

  it("shows the 5-hour USD limit inside create subscription settings only", async () => {
    const wrapper = await mountView();

    await wrapper.get('[data-tour="groups-create-btn"]').trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("Billing type");
    expect(wrapper.text()).not.toContain("5-hour USD limit");

    const billingTypeSelects = wrapper.findAll("select").filter((select) =>
      select.element.outerHTML.includes("standard") &&
      select.element.outerHTML.includes("subscription"),
    );
    await billingTypeSelects.at(-1)!.setValue("subscription");
    await flushPromises();

    const rateLimitIndex = wrapper.text().indexOf("5-hour USD limit");
    expect(rateLimitIndex).toBeGreaterThan(-1);
    expect(rateLimitIndex).toBeGreaterThan(wrapper.text().indexOf("Billing type"));
    expect(rateLimitIndex).toBeLessThan(wrapper.text().indexOf("Daily limit"));
  });

  it("submits zero 5-hour limit for standard groups and configured value for subscriptions", async () => {
    const wrapper = await mountView();

    await wrapper.get('[data-tour="groups-create-btn"]').trigger("click");
    await wrapper.get('input[required]').setValue("standard-new");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(createGroup).toHaveBeenLastCalledWith(
      expect.objectContaining({
        subscription_type: "standard",
        rate_limit_5h: 0,
      }),
    );

    await wrapper.get('[data-tour="groups-create-btn"]').trigger("click");
    await wrapper.get('input[required]').setValue("subscription-new");
    const billingTypeSelects = wrapper.findAll("select").filter((select) =>
      select.element.outerHTML.includes("standard") &&
      select.element.outerHTML.includes("subscription"),
    );
    await billingTypeSelects.at(-1)!.setValue("subscription");
    await flushPromises();
    await wrapper.get('input[placeholder="admin.groups.form.rateLimit5hPlaceholder"]').setValue("12.5");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(createGroup).toHaveBeenLastCalledWith(
      expect.objectContaining({
        subscription_type: "subscription",
        rate_limit_5h: 12.5,
      }),
    );
  });

  it("hides the edit 5-hour limit for standard groups and clears it on save", async () => {
    const wrapper = await mountView();

    await wrapper
      .findAll('[data-test="row-actions"] button')
      .find((button) => button.text() === "Edit")!
      .trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("Billing type");
    expect(wrapper.text()).not.toContain("5-hour USD limit");

    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(updateGroup).toHaveBeenLastCalledWith(
      1,
      expect.objectContaining({
        subscription_type: "standard",
        rate_limit_5h: 0,
      }),
    );
  });

  it("shows the edit 5-hour limit for subscription groups", async () => {
    const wrapper = await mountView();

    await wrapper
      .findAll('[data-test="row-actions"] button')
      .filter((button) => button.text() === "Edit")[1]
      .trigger("click");
    await flushPromises();

    const rateLimitIndex = wrapper.text().indexOf("5-hour USD limit");
    expect(rateLimitIndex).toBeGreaterThan(-1);
    expect(rateLimitIndex).toBeGreaterThan(wrapper.text().indexOf("Billing type"));
    expect(rateLimitIndex).toBeLessThan(wrapper.text().indexOf("Daily limit"));
  });
});
