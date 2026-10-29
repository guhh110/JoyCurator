<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import {
  AccordionRoot,
  AccordionItem,
  AccordionHeader,
  AccordionTrigger,
  AccordionContent,
} from 'reka-ui'
import { ChevronDown, Layers3 } from '@lucide/vue'
import { navigation, groupForRoute } from '../../app/navigation'
defineEmits<{ navigate: [] }>()
const route = useRoute()
const expanded = ref(groupForRoute(route.name))
watch(
  () => route.name,
  (name) => {
    expanded.value = groupForRoute(name)
  },
)
</script>
<template>
  <div class="sidebar-inner">
    <RouterLink
      class="brand sidebar-brand"
      :to="{ name: 'workbench' }"
      @click="$emit('navigate')"
      ><div class="brand-symbol"><Layers3 :size="23" /></div>
      <span>JoyCurator<small>你的媒体整理空间</small></span></RouterLink
    >
    <nav aria-label="分组导航">
      <AccordionRoot
        v-model="expanded"
        type="single"
        collapsible
        class="navigation-accordion"
        ><AccordionItem
          v-for="group in navigation"
          :key="group.id"
          :value="group.id"
          class="navigation-group"
          ><AccordionHeader
            ><AccordionTrigger class="navigation-trigger"
              ><component :is="group.icon" :size="17" /><span>{{
                group.title
              }}</span
              ><ChevronDown
                class="navigation-chevron"
                :size="15" /></AccordionTrigger></AccordionHeader
          ><AccordionContent class="navigation-content"
            ><RouterLink
              v-for="item in group.items"
              :key="item.name"
              :to="{ name: item.name }"
              class="navigation-link"
              :class="{ active: route.name === item.name }"
              :aria-current="route.name === item.name ? 'page' : undefined"
              @click="$emit('navigate')"
              ><component :is="item.icon" :size="16" />{{
                item.title
              }}</RouterLink
            ></AccordionContent
          ></AccordionItem
        ></AccordionRoot
      >
    </nav>
    <footer class="sidebar-foot">
      <span class="status-dot" />本地文件 · 先预览再整理
    </footer>
  </div>
</template>
<style scoped>
.sidebar-inner {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 22px 14px;
  background: #fff;
  min-width: 0;
}
.sidebar-brand {
  font-size: 19px;
  gap: 10px;
  padding: 0 8px 27px;
}
.sidebar-brand span {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sidebar-brand small {
  font-size: 10px;
  letter-spacing: 0.5px;
  color: var(--muted);
  font-weight: 400;
}
.navigation-group {
  margin-bottom: 10px;
}
.navigation-group h3 {
  margin: 0;
}
.navigation-trigger {
  width: 100%;
  border: 0;
  background: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 12px;
  font-size: 13px;
  color: #747180;
  text-align: left;
  border-radius: 8px;
  cursor: pointer;
}
.navigation-trigger:hover {
  background: #f7f6fb;
}
.navigation-trigger span {
  flex: 1;
  font-weight: 600;
}
.navigation-chevron {
  transition: transform 0.2s;
}
.navigation-trigger[data-state='open'] .navigation-chevron {
  transform: rotate(180deg);
}
.navigation-content {
  overflow: hidden;
  display: grid;
  gap: 3px;
  padding: 3px 0;
}
.navigation-content[data-state='closed'] {
  padding: 0;
}
.navigation-link {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  padding: 12px 12px 12px 22px;
  color: #898494;
  font-size: 12px;
  border-radius: 8px;
  min-height: 42px;
}
.navigation-link:hover {
  background: #f7f6fb;
  color: var(--purple);
}
.navigation-link.active {
  background: #eee9fc;
  color: var(--purple);
  font-weight: 600;
  box-shadow: inset 3px 0 var(--purple);
}
.sidebar-foot {
  margin-top: auto;
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 10px;
  color: #aaa5b4;
  padding: 22px 8px 2px;
}
</style>
