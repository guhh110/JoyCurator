<script setup lang="ts">
import { useRoute, RouterLink } from 'vue-router'
import { Layers3 } from '@lucide/vue'
import { navigation, menuRoute } from '../../app/navigation'
defineEmits<{ navigate: [] }>()
const route = useRoute()
</script>
<template>
  <div class="sidebar-inner">
    <RouterLink
      class="brand sidebar-brand"
      :to="{ name: 'automatic-recognition' }"
      @click="$emit('navigate')"
      ><div class="brand-symbol"><Layers3 :size="23" /></div>
      <span>JoyCurator<small>你的媒体整理空间</small></span></RouterLink
    >
    <nav aria-label="分组导航">
      <section
          v-for="group in navigation"
          :key="group.id"
          class="navigation-group"
          ><h3 class="navigation-title">{{ group.title }}</h3>
          <div class="navigation-content"
            ><RouterLink
              v-for="item in group.items"
              :key="item.name"
              :to="{ name: item.name }"
              class="navigation-link"
              :class="{ active: menuRoute(route.name) === item.name }"
              :aria-current="menuRoute(route.name) === item.name ? 'page' : undefined"
              @click="$emit('navigate')"
              ><component :is="item.icon" :size="16" />{{
                item.title
              }}</RouterLink
            ></div>
          </section>
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
  flex-shrink: 0;
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
.sidebar-inner > nav { flex:1; min-height:0; overflow-y:auto; overscroll-behavior:contain; }
.navigation-title { padding:12px 12px 6px; color:var(--muted); font-size:11px; font-weight:600; letter-spacing:1px; }
.navigation-group h3 {
  margin: 0;
}
.navigation-content {
  overflow: hidden;
  display: grid;
  gap: 3px;
  padding: 3px 0;
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
  flex-shrink: 0;
  margin-top: auto;
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 10px;
  color: #aaa5b4;
  padding: 22px 8px 2px;
}
</style>
