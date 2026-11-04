/* ============================================================================
 *  我不是魔王（Im No Demonlord）本地作弊 Mod
 *  Local-Cheat for Im No Demonlord  ·  v1.0.0
 *
 *  原理：
 *    Electron 版游戏使用 Cocos Creator 3.x 的 web-mobile 构建，
 *    所有游戏逻辑模块都注册在 SystemJS 的模块表里，可以直接用
 *      System.get('chunks:///_virtual/<Module>.ts')
 *    拿到类和单例，不需要修改 25MB 的混淆 bundle。
 *
 *  这个脚本只做三件事：
 *    1. 在页面里叠加一个 DOM 作弊面板（Cocos 画布之上）；
 *    2. 用游戏自己的 API / 原型包装实现各项功能；
 *    3. 用 window 捕获阶段的键盘事件处理 F2/F3... 热键。
 *
 *  本文件由加载器在运行时通过 CDP 注入，或由加载器的离线安装写入
 *  app.asar 的 web-mobile/cheat.js。
 *  仅供单机自用与学习，请勿用于联机或商业用途。
 * ========================================================================== */
(function () {
  'use strict';

  if (window.__IMND_CHEAT__) return;
  window.__IMND_CHEAT__ = true;

  var VERSION = '1.0.0';
  var MOD_PREFIX = 'chunks:///_virtual/';
  window.__IMND_CHEAT_VERSION__ = VERSION;

  /* ------------------------------------------------------------------ *
   * 运行时状态
   * ------------------------------------------------------------------ */
  var state = {
    money: false,          // 无限金币
    soul: false,           // 无限灵魂钱币
    expX10: false,         // 经验 ×10
    freeShop: false,       // 免费商店
    infiniteRefresh: false,// 无限刷新
    god: false,            // 无敌
    autoHeal: false,       // 自动回血
    bigHp: false,          // 生命上限 99,999
    oneHit: false,         // 秒杀（伤害 ×1e9）
    autoKill: false,       // 自动清怪
    autoPick: false,       // 自动拾取
    speedX2: false,        // 移动速度 ×2
    magnet: false,         // 拾取范围 ×5
    skipAnim: false,       // 跳过过场动画
    timeScale: 1           // 游戏速度
  };

  // 开启前的原值，关闭时还原
  var prev = {
    god: false,
    infiniteRefresh: false,
    skipAnim: false
  };

  var instantiated = {
    hooks: false,
    ui: false,
    panelOpen: false
  };

  // 刚注入时游戏数据可能还没加载完，持续清理几十秒上一次留下的游戏侧开关
  var bootCleanUntil = 0;

  var hooks = [];          // {proto, name, orig}
  var toastTimer = null;

  var LABELS = {
    money: '无限金币',
    soul: '无限灵魂钱币',
    expX10: '经验 ×10',
    freeShop: '免费商店',
    infiniteRefresh: '无限刷新',
    god: '无敌模式',
    autoHeal: '自动回血',
    bigHp: '生命上限 99,999',
    oneHit: '秒杀敌人',
    autoKill: '自动清怪',
    autoPick: '自动拾取',
    speedX2: '移动速度 ×2',
    magnet: '拾取范围 ×5',
    skipAnim: '跳过过场动画'
  };

  /* ------------------------------------------------------------------ *
   * 工具函数
   * ------------------------------------------------------------------ */
  function safe(fn, fallback) {
    try { return fn(); } catch (e) { return fallback; }
  }

  function mod(name) {
    if (typeof System === 'undefined' || !System.get) return null;
    try { return System.get(MOD_PREFIX + name + '.ts'); } catch (e) { return null; }
  }

  function inst(name, singleton) {
    var m = mod(name);
    if (!m) return null;
    return singleton ? (m[singleton] && m[singleton].Instance) : m[singleton];
  }

  function cdm() { return inst('CharaDataManager', 'CharaDataManager'); }
  function bpm() { return inst('BattleProcessManager', 'BattleProcessManager'); }
  function bom() { return inst('BattleObjectManager', 'BattleObjectManager'); }
  function gm()  { return inst('GameManager', 'GameManager'); }
  function dm()  { return inst('DataManager', 'DataManager'); }
  function uiMgr() { return inst('UIManager', 'UIManager'); }

  function isBattle() {
    var g = gm();
    return !!(g && safe(function () { return g.isInBattle(); }, false));
  }

  function n(v) {
    return (typeof v === 'number' && isFinite(v)) ? v : 0;
  }

  function fmt(v) {
    v = Math.round(n(v));
    return String(v).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  }

  function protoHook(proto, name, factory) {
    if (!proto || typeof proto[name] !== 'function') return false;
    var orig = proto[name];
    if (orig.__imndWrapped) return true;
    var wrapped = factory(orig);
    wrapped.__imndWrapped = true;
    wrapped.__imndOrig = orig;
    proto[name] = wrapped;
    hooks.push({ proto: proto, name: name, orig: orig });
    return true;
  }

  /* ------------------------------------------------------------------ *
   * 原型包装（只包一次，运行中通过 state 开关）
   * ------------------------------------------------------------------ */
  function installHooks() {
    if (instantiated.hooks) return;

    var cdmMod = mod('CharaDataManager');
    var ecMod = mod('EnemyController');
    var shopItemMod = mod('BattleShopItem');
    var shopCtrlMod = mod('UIBattleShopController');
    if (!cdmMod || !ecMod || !shopItemMod || !shopCtrlMod) return;

    var CDM = cdmMod.CharaDataManager;
    var EC = ecMod.EnemyController;
    var BSI = shopItemMod.BattleShopItem;
    var UBS = shopCtrlMod.UIBattleShopController;

    // 经验倍率
    protoHook(CDM.prototype, 'expUp', function (orig) {
      return function (exp) {
        if (state.expX10 && typeof exp === 'number' && exp > 0 && isFinite(exp)) {
          exp = exp * 10;
        }
        return orig.call(this, exp);
      };
    });

    // 生命上限
    protoHook(CDM.prototype, 'getMaxHp', function (orig) {
      return function (index) {
        var v = orig.call(this, index);
        if (state.bigHp && isFinite(v)) return Math.max(v, 99999);
        return v;
      };
    });

    // 移动速度
    protoHook(CDM.prototype, 'getMoveSpeed', function (orig) {
      return function (index) {
        var v = orig.call(this, index);
        if (state.speedX2 && isFinite(v)) return v * 2;
        return v;
      };
    });

    // 拾取范围
    protoHook(CDM.prototype, 'getCollectScale', function (orig) {
      return function (index) {
        var v = orig.call(this, index);
        if (state.magnet && isFinite(v)) return Math.max(v, 5);
        return v;
      };
    });

    // 秒杀：把玩家造成的伤害直接拉满，并补上死亡结算
    // （游戏的 damaged() 只扣血，不会自己触发 startDead，需要在这里补刀）
    protoHook(EC.prototype, 'damaged', function (orig) {
      return function (charaIndex, damage) {
        if (!state.oneHit) return orig.apply(this, arguments);
        if (typeof damage === 'number' && damage > 0 && isFinite(damage)) {
          arguments[1] = 1e9;
        }
        var ret = orig.apply(this, arguments);
        safe(function () {
          if (!this.isDead() && this.getCurHp() <= 0) this.startDead();
        }.bind(this));
        return ret;
      };
    });

    // 免费商店：购买时把结算价临时改成 0
    protoHook(BSI.prototype, '_onClickItemBuy', function (orig) {
      return function () {
        if (!state.freeShop) return orig.apply(this, arguments);
        var old = this._showedRealPrice;
        this._showedRealPrice = 0;
        try {
          return orig.apply(this, arguments);
        } finally {
          this._showedRealPrice = old;
        }
      };
    });

    // 免费刷新（免费商店开启时）
    protoHook(UBS.prototype, '_onClickRefreshButton', function (orig) {
      return function (index) {
        if (!state.freeShop || !this._refreshRealPrice) {
          return orig.apply(this, arguments);
        }
        var old = this._refreshRealPrice[index];
        this._refreshRealPrice[index] = 0;
        try {
          return orig.apply(this, arguments);
        } finally {
          this._refreshRealPrice[index] = old;
        }
      };
    });

    instantiated.hooks = true;
    bootCleanUntil = Date.now() + 20000;
    clearPersistedFlags();
    console.log('[IMND-Cheat] hooks installed');
  }

  /* ------------------------------------------------------------------ *
   * 功能实现
   * ------------------------------------------------------------------ */
  function topUpMoney() {
    var m = cdm();
    if (!m) return;
    var cur = n(m.getCurMoney(0));
    if (cur < 999999) {
      safe(function () { m.tryChangeMoney(0, 999999 - cur); });
    }
  }

  function topUpSoul() {
    var m = cdm();
    if (!m || !m.getCurInfluence) return;
    var cur = n(m.getCurInfluence(0));
    if (cur < 99999) {
      safe(function () { m.tryChangeInfluence(0, 99999 - cur); });
    }
  }

  function autoHeal() {
    var m = cdm();
    if (!m) return;
    var data = safe(function () { return m.getCharaData(0); }, null);
    if (data && data.forceSetHpPercentage) {
      safe(function () { data.forceSetHpPercentage(1); });
    }
  }

  function killAll() {
    if (!isBattle()) return;
    var mgr = bom();
    if (!mgr) return;
    var list = safe(function () { return mgr._enemyObjs; }, null);
    var count = safe(function () { return mgr.getEnemyNum(); }, 0);
    if (!count) return;
    // 先照常结算一次伤害（保留伤害数字/吸血等效果）
    safe(function () { mgr.dealDamageToAllEnemies(0, 1e9); });
    // 再兜底：damaged() 不会自动走死亡流程，血量为 0 的敌人手动结算
    if (list && list.length) {
      for (var i = list.length - 1; i >= 0; i--) {
        (function (e) {
          safe(function () {
            if (e && !e.isDead() && e.getCurHp() <= 0) e.startDead();
          });
        })(list[i]);
      }
    }
  }

  function autoPick() {
    var mgr = bom();
    if (!mgr || !mgr.dealForceMagnet) return;
    safe(function () { mgr.dealForceMagnet(); });
  }

  function ensureInfiniteRefresh() {
    var d = dm();
    if (!d) return;
    if (!safe(function () { return d.isInfiniteRefreshOn(); }, false)) {
      safe(function () { d.setInfiniteRefresh(true); });
    }
  }

  // 上一次运行留下的游戏侧开关（存档里会保留），启动时清掉，保证“默认全关”
  function clearPersistedFlags() {
    var d = dm();
    if (!d) return;
    safe(function () { if (d.isInfiniteRefreshOn && d.isInfiniteRefreshOn()) d.setInfiniteRefresh(false); });
    safe(function () { if (d.setSkipAnim) d.setSkipAnim(false); });
    safe(function () { if (d.setHurtGod) d.setHurtGod(false); });
  }

  function ensureGod() {
    var m = cdm();
    if (!m) return;
    if (!safe(function () { return m.getGodMode(); }, false)) {
      safe(function () { m.setGodMode(true); });
    }
  }

  function ensureTimeScale() {
    var t = mod('TimeScaleUtils');
    if (!t || !t.TimeScaleUtils || !t.TimeScaleUtils.setTimeScale) return;
    var cur = safe(function () { return t.TimeScaleUtils.getTimeScale(); }, state.timeScale);
    if (Math.abs(n(cur) - state.timeScale) > 0.001) {
      safe(function () { t.TimeScaleUtils.setTimeScale(state.timeScale); });
    }
  }

  // 商店价格显示为 0
  function shopController() {
    var u = uiMgr();
    if (!u) return null;
    return safe(function () { return u.getUIControllerByName('UIBattleShopController'); }, null);
  }

  function paintShopLabels() {
    var c = shopController();
    if (!c) return;
    var items = c._battleShopItems;
    if (items) {
      for (var i = 0; i < items.length; i++) {
        var it = items[i];
        if (!it || !it._realPriceLabel) continue;
        safe(function () { it._realPriceLabel.string = '0'; });
      }
    }
    var labels = c._refreshRealPriceLabel;
    if (labels) {
      for (var j = 0; j < labels.length; j++) {
        if (labels[j]) safe(function () { labels[j].string = '0'; });
      }
    }
  }

  function restoreShopLabels() {
    var c = shopController();
    if (!c) return;
    var items = c._battleShopItems;
    if (items) {
      for (var i = 0; i < items.length; i++) {
        var it = items[i];
        if (!it || !it._realPriceLabel) continue;
        safe(function () { it._realPriceLabel.string = String(n(it._showedRealPrice)); });
      }
    }
    var labels = c._refreshRealPriceLabel;
    if (labels && c._refreshRealPrice) {
      for (var j = 0; j < labels.length; j++) {
        if (labels[j]) safe(function () { labels[j].string = String(n(c._refreshRealPrice[j])); });
      }
    }
  }

  /* ------------------------------------------------------------------ *
   * 主循环（250ms）
   * ------------------------------------------------------------------ */
  function tick() {
    installHooks();

    if (state.money) topUpMoney();
    if (state.soul) topUpSoul();
    if (state.god) ensureGod();
    if (state.autoHeal && isBattle()) autoHeal();
    if (state.autoKill) killAll();
    if (state.autoPick) autoPick();
    if (state.infiniteRefresh) ensureInfiniteRefresh();
    else if (Date.now() < bootCleanUntil) clearPersistedFlags();
    if (state.freeShop) paintShopLabels();
    ensureTimeScale();

    if (instantiated.panelOpen) updateStats();
  }

  function updateStats() {
    var moneyEl = document.getElementById('imnd-st-money');
    if (!moneyEl) return;

    var m = cdm();
    var p = bpm();
    var b = bom();

    var money = m ? n(safe(function () { return m.getCurMoney(0); }, 0)) : 0;
    var soul = (m && m.getCurInfluence) ? n(safe(function () { return m.getCurInfluence(0); }, 0)) : 0;
    var level = m ? n(safe(function () { return m.getLevel(); }, 0)) : 0;
    var hp = m ? n(safe(function () { return m.getHp(0); }, 0)) : 0;
    var maxHp = m ? n(safe(function () { return m.getMaxHp(0); }, 0)) : 0;
    var day = p ? n(safe(function () { return p.getCurDayCount(); }, 0)) : 0;
    var remain = p ? n(safe(function () { return p.getProcessRemainTime(); }, 0)) : 0;
    var enemies = b ? n(safe(function () { return b.getEnemyNum(); }, 0)) : 0;

    moneyEl.textContent = fmt(money);
    var soulEl = document.getElementById('imnd-st-soul');
    if (soulEl) soulEl.textContent = fmt(soul);
    document.getElementById('imnd-st-level').textContent = fmt(level);
    document.getElementById('imnd-st-hp').textContent = fmt(hp) + ' / ' + fmt(maxHp);
    document.getElementById('imnd-st-day').textContent = day ? ('第 ' + day + ' 天') : '—';
    document.getElementById('imnd-st-time').textContent = isBattle() ? (remain.toFixed(1) + ' 秒') : '—';
    document.getElementById('imnd-st-enemy').textContent = isBattle() ? fmt(enemies) : '—';
  }

  /* ------------------------------------------------------------------ *
   * 开关逻辑
   * ------------------------------------------------------------------ */
  function setToggle(key, on, silent) {
    on = !!on;
    if (state[key] === on) {
      syncCheckbox(key);
      return;
    }

    // 关闭时需要还原的副作用
    if (!on) {
      if (key === 'god') {
        var m = cdm();
        if (m) safe(function () { m.setGodMode(prev.god); });
      } else if (key === 'infiniteRefresh') {
        var d = dm();
        if (d) safe(function () { d.setInfiniteRefresh(prev.infiniteRefresh); });
      } else if (key === 'skipAnim') {
        var d2 = dm();
        if (d2) safe(function () { d2.setSkipAnim(prev.skipAnim); });
      } else if (key === 'bigHp') {
        var m2 = cdm();
        if (m2) {
          var data = safe(function () { return m2.getCharaData(0); }, null);
          if (data) {
            var max = n(safe(function () { return m2.getMaxHp(0); }, 0));
            var hp = n(safe(function () { return m2.getHp(0); }, 0));
            if (max > 0 && hp > max) safe(function () { data.forceSetHp(max); });
          }
        }
      } else if (key === 'freeShop') {
        restoreShopLabels();
      }
    }

    state[key] = on;

    // 开启时的副作用
    if (on) {
      if (key === 'god') {
        var m3 = cdm();
        if (m3) {
          prev.god = safe(function () { return m3.getGodMode(); }, false);
          safe(function () { m3.setGodMode(true); });
        }
      } else if (key === 'infiniteRefresh') {
        var d3 = dm();
        if (d3) {
          prev.infiniteRefresh = safe(function () { return d3.isInfiniteRefreshOn(); }, false);
          safe(function () { d3.setInfiniteRefresh(true); });
        }
      } else if (key === 'skipAnim') {
        var d4 = dm();
        if (d4) {
          prev.skipAnim = safe(function () { return d4.isSkipAnimOn(); }, false);
          safe(function () { d4.setSkipAnim(true); });
        }
      } else if (key === 'bigHp') {
        autoHeal();
      } else if (key === 'autoHeal') {
        autoHeal();
      }
    }

    syncCheckbox(key);
    if (!silent) toast(LABELS[key] + '：' + (on ? '开启' : '关闭'));
  }

  function syncCheckbox(key) {
    var el = document.querySelector('input[data-toggle="' + key + '"]');
    if (el && el.checked !== !!state[key]) el.checked = !!state[key];
  }

  function syncAllCheckboxes() {
    Object.keys(LABELS).forEach(syncCheckbox);
    var sel = document.getElementById('imnd-speed');
    if (sel) sel.value = String(state.timeScale);
  }

  function resetAll() {
    Object.keys(LABELS).forEach(function (k) { setToggle(k, false, true); });
    state.timeScale = 1;
    var t = mod('TimeScaleUtils');
    if (t && t.TimeScaleUtils && t.TimeScaleUtils.setTimeScale) {
      safe(function () { t.TimeScaleUtils.setTimeScale(1); });
    }
    syncAllCheckboxes();
    toast('已关闭全部作弊功能');
  }

  /* ------------------------------------------------------------------ *
   * 操作按钮
   * ------------------------------------------------------------------ */
  function killWave() {
    killAll();
    var mgr = bom();
    var count = mgr ? n(safe(function () { return mgr.getEnemyNum(); }, 0)) : 0;
    toast(count > 0 ? ('已对 ' + count + ' 个敌人造成秒杀伤害') : '当前没有敌人');
  }

  function endWave() {
    var p = bpm();
    if (!p) { toast('游戏尚未加载'); return; }
    var end = safe(function () { return p.getProcessEndTime(); }, 0);
    if (!end) { toast('当前不在战斗中'); return; }
    safe(function () { p._gameProcessTime = end; });
    toast('已把本波计时推到结束');
  }

  function levelUp() {
    var m = cdm();
    if (!m) { toast('游戏尚未加载'); return; }
    safe(function () { m.executeLevelUp(); });
    toast('已升一级');
  }

  function addMoney() {
    var m = cdm();
    if (!m) { toast('游戏尚未加载'); return; }
    safe(function () { m.tryChangeMoney(0, 10000); });
    toast('金币 +10,000');
  }

  function addSoul() {
    var m = cdm();
    if (!m || !m.tryChangeInfluence) { toast('游戏尚未加载'); return; }
    safe(function () { m.tryChangeInfluence(0, 1000); });
    toast('灵魂钱币 +1,000');
  }

  function addExp() {
    var m = cdm();
    if (!m) { toast('游戏尚未加载'); return; }
    safe(function () { m.expUp(1000); });
    toast('经验 +1,000');
  }

  function setSpeed(v) {
    state.timeScale = v;
    var t = mod('TimeScaleUtils');
    if (t && t.TimeScaleUtils && t.TimeScaleUtils.setTimeScale) {
      safe(function () { t.TimeScaleUtils.setTimeScale(v); });
    }
    toast('游戏速度：' + v + '×');
  }

  /* ------------------------------------------------------------------ *
   * 面板 UI
   * ------------------------------------------------------------------ */
  var SECTIONS = [
    {
      title: '资源',
      rows: [
        { key: 'money', name: '无限金币', desc: '金币自动补满至 999,999', hot: 'F3' },
        { key: 'soul', name: '无限灵魂钱币', desc: '灵魂钱币自动补满至 99,999', hot: 'F9' },
        { key: 'expX10', name: '经验 ×10', desc: '获得的经验放大 10 倍' },
        { key: 'freeShop', name: '免费商店', desc: '商品与刷新按 0 元结算', hot: 'F7' },
        { key: 'infiniteRefresh', name: '无限刷新', desc: '商店刷新不花钱、不涨价', hot: 'F8' }
      ]
    },
    {
      title: '战斗',
      rows: [
        { key: 'god', name: '无敌模式', desc: '不会受到任何伤害', hot: 'F4' },
        { key: 'autoHeal', name: '自动回血', desc: '战斗中血量始终保持满值' },
        { key: 'bigHp', name: '生命上限 99,999', desc: '放大生命上限并回满' },
        { key: 'oneHit', name: '秒杀敌人', desc: '玩家造成的伤害提升到 1e9' },
        { key: 'autoKill', name: '自动清怪', desc: '每 0.25 秒清掉场上所有敌人', hot: 'F6' },
        { key: 'autoPick', name: '自动拾取', desc: '持续把掉落物吸到角色身边' },
        { key: 'speedX2', name: '移动速度 ×2', desc: '角色移动速度翻倍' },
        { key: 'magnet', name: '拾取范围 ×5', desc: '扩大金币/经验的拾取范围' },
        { key: 'skipAnim', name: '跳过过场动画', desc: '略过部分进场/结算动画' }
      ]
    }
  ];

  function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function buildPanel() {
    var root = document.createElement('div');
    root.id = 'imnd-cheat-root';

    var rowsHtml = SECTIONS.map(function (sec) {
      return '<div class="imnd-sec">' +
        '<div class="imnd-sec-title">' + esc(sec.title) + '</div>' +
        '<div class="imnd-grid">' +
        sec.rows.map(function (r) {
          return '<label class="imnd-row">' +
            '<span class="imnd-meta"><span class="imnd-name">' + esc(r.name) + '</span>' +
            '<span class="imnd-desc">' + esc(r.desc) + '</span></span>' +
            (r.hot ? '<kbd class="imnd-key">' + esc(r.hot) + '</kbd>' : '') +
            '<span class="imnd-switch"><input type="checkbox" data-toggle="' + r.key + '"><i></i></span>' +
            '</label>';
        }).join('') +
        '</div></div>';
    }).join('');

    root.innerHTML =
      '<style>' +
      '#imnd-cheat-root,#imnd-cheat-root *{box-sizing:border-box;}' +
      '#imnd-cheat-root{position:fixed;inset:0;pointer-events:none;z-index:2147483600;' +
        'font-family:"Microsoft YaHei","PingFang SC","Segoe UI",system-ui,sans-serif;' +
        'color:#f3e9d8;-webkit-font-smoothing:antialiased;}' +
      '#imnd-panel{pointer-events:auto;position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);' +
        'width:min(720px,94vw);max-height:92vh;display:flex;flex-direction:column;' +
        'background:linear-gradient(180deg,rgba(30,22,20,.97),rgba(18,14,14,.98));' +
        'border:1px solid rgba(255,182,72,.45);border-radius:16px;overflow:hidden;' +
        'box-shadow:0 24px 80px rgba(0,0,0,.75),0 0 0 1px rgba(0,0,0,.6) inset,0 0 42px rgba(255,150,40,.10);}' +
      '#imnd-panel.imnd-hidden{display:none;}' +
      '.imnd-head{display:flex;align-items:center;gap:10px;padding:14px 18px 12px;' +
        'background:linear-gradient(90deg,rgba(255,160,50,.16),rgba(255,160,50,0));' +
        'border-bottom:1px solid rgba(255,182,72,.22);}' +
      '.imnd-title{font-size:17px;font-weight:800;letter-spacing:.5px;color:#ffcf87;' +
        'text-shadow:0 1px 0 rgba(0,0,0,.6);}' +
      '.imnd-sub{font-size:11px;color:#a89480;margin-top:2px;}' +
      '.imnd-head-main{flex:1;min-width:0;}' +
      '.imnd-reset{flex:none;height:28px;padding:0 10px;border-radius:8px;cursor:pointer;font-size:11.5px;font-weight:700;' +
        'border:1px solid rgba(255,120,90,.38);background:rgba(255,90,60,.10);color:#ffb9a8;}' +
      '.imnd-reset:hover{background:rgba(255,90,60,.22);}' +
      '.imnd-close{width:30px;height:30px;border-radius:8px;border:1px solid rgba(255,255,255,.12);' +
        'background:rgba(255,255,255,.05);color:#f3e9d8;font-size:18px;line-height:1;cursor:pointer;}' +
      '.imnd-close:hover{background:rgba(255,90,70,.28);border-color:rgba(255,90,70,.5);}' +
      '.imnd-stats{display:grid;grid-template-columns:repeat(7,1fr);gap:1px;background:rgba(255,255,255,.06);' +
        'border-bottom:1px solid rgba(255,182,72,.18);}' +
      '.imnd-stat{background:rgba(0,0,0,.25);padding:8px 4px 7px;text-align:center;min-width:0;}' +
      '.imnd-stat b{display:block;font-size:14px;color:#ffe0ad;font-weight:700;white-space:nowrap;' +
        'overflow:hidden;text-overflow:ellipsis;}' +
      '.imnd-stat span{display:block;font-size:10px;color:#9d8b78;margin-top:2px;}' +
      '.imnd-scroll{overflow-y:auto;padding:6px 14px 10px;scrollbar-width:thin;' +
        'scrollbar-color:rgba(255,182,72,.45) transparent;}' +
      '.imnd-scroll::-webkit-scrollbar{width:8px;}' +
      '.imnd-scroll::-webkit-scrollbar-thumb{background:rgba(255,182,72,.35);border-radius:8px;}' +
      '.imnd-sec{padding:8px 0 2px;}' +
      '.imnd-grid{display:grid;grid-template-columns:1fr 1fr;gap:0 8px;}' +
      '.imnd-sec-title{font-size:11px;font-weight:700;letter-spacing:2px;color:#c99a58;' +
        'margin:2px 4px 6px;}' +
      '.imnd-row{display:flex;align-items:center;gap:8px;padding:7px 9px;border-radius:10px;' +
        'cursor:pointer;user-select:none;}' +
      '.imnd-row:hover{background:rgba(255,182,72,.07);}' +
      '.imnd-meta{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px;}' +
      '.imnd-name{font-size:13.5px;font-weight:600;color:#f5ecdd;}' +
      '.imnd-desc{font-size:11px;color:#9f8d79;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}' +
      '.imnd-key{flex:none;font-family:Consolas,monospace;font-size:10.5px;color:#ffbd68;' +
        'background:rgba(255,182,72,.10);border:1px solid rgba(255,182,72,.28);border-radius:5px;' +
        'padding:2px 5px;}' +
      '.imnd-switch{flex:none;width:40px;height:22px;border-radius:999px;position:relative;' +
        'background:rgba(255,255,255,.10);border:1px solid rgba(255,255,255,.14);transition:.18s;}' +
      '.imnd-switch i{position:absolute;left:2px;top:2px;width:16px;height:16px;border-radius:50%;' +
        'background:#8d8173;transition:.18s;box-shadow:0 1px 2px rgba(0,0,0,.5);}' +
      '.imnd-switch input{position:absolute;opacity:0;width:0;height:0;}' +
      '.imnd-switch input:checked+i{left:20px;background:#ffb648;}' +
      '.imnd-switch:has(input:checked){background:rgba(255,182,72,.25);border-color:rgba(255,182,72,.55);}' +
      '.imnd-speed{display:flex;align-items:center;gap:10px;padding:8px 10px;}' +
      '.imnd-speed label{flex:1;font-size:13.5px;font-weight:600;}' +
      '.imnd-speed .imnd-desc{display:block;margin-top:2px;font-weight:400;}' +
      '.imnd-speed select{background:#2a211c;color:#ffe0ad;border:1px solid rgba(255,182,72,.35);' +
        'border-radius:8px;padding:5px 8px;font-size:12px;outline:none;}' +
      '.imnd-actions-wrap{flex:none;border-top:1px solid rgba(255,182,72,.16);padding:4px 14px 8px;background:rgba(0,0,0,.18);}' +
      '.imnd-actions{display:grid;grid-template-columns:1fr 1fr;gap:7px;padding:4px 0 2px;}' +
      '.imnd-btn{display:flex;align-items:center;justify-content:center;gap:6px;padding:7px 8px;' +
        'border-radius:9px;border:1px solid rgba(255,182,72,.28);background:rgba(255,182,72,.09);' +
        'color:#f7ead8;font-size:12.5px;font-weight:600;cursor:pointer;transition:.15s;}' +
      '.imnd-btn:hover{background:rgba(255,182,72,.18);border-color:rgba(255,182,72,.5);}' +
      '.imnd-btn.imnd-wide{grid-column:1/3;}' +
      '.imnd-btn kbd{font-family:Consolas,monospace;font-size:10px;color:#ffbd68;opacity:.9;}' +
      '.imnd-foot{padding:9px 18px 11px;font-size:10.5px;color:#8e7d6b;' +
        'border-top:1px solid rgba(255,182,72,.16);text-align:center;}' +
      '#imnd-fab{pointer-events:auto;position:fixed;left:14px;bottom:14px;z-index:2147483600;' +
        'padding:7px 12px;border-radius:999px;background:rgba(20,15,14,.82);' +
        'border:1px solid rgba(255,182,72,.38);color:#ffcf87;font-size:11.5px;font-weight:700;' +
        'cursor:pointer;user-select:none;backdrop-filter:blur(6px);' +
        'font-family:"Microsoft YaHei",system-ui,sans-serif;box-shadow:0 4px 18px rgba(0,0,0,.45);}' +
      '#imnd-fab:hover{background:rgba(255,182,72,.18);}' +
      '#imnd-toast{pointer-events:none;position:fixed;left:50%;top:4px;transform:translateX(-50%);' +
        'z-index:2147483601;padding:6px 14px;border-radius:999px;background:rgba(18,13,12,.92);' +
        'border:1px solid rgba(255,182,72,.45);color:#ffe0ad;font-size:12.5px;font-weight:600;' +
        'opacity:0;transition:opacity .18s;box-shadow:0 8px 30px rgba(0,0,0,.5);}' +
      '#imnd-toast.imnd-show{opacity:1;}' +
      '@media (max-width:680px){.imnd-grid{grid-template-columns:1fr;}.imnd-stats{grid-template-columns:repeat(4,1fr);}' +
        '.imnd-actions{grid-template-columns:1fr;}.imnd-btn.imnd-wide{grid-column:auto;}}' +
      '</style>' +
      '<div id="imnd-panel" class="imnd-hidden">' +
        '<div class="imnd-head">' +
          '<div class="imnd-head-main">' +
            '<div class="imnd-title">我不是魔王 · 本地作弊</div>' +
            '<div class="imnd-sub">Local-Cheat v' + VERSION + ' · F2 显示 / 隐藏</div>' +
          '</div>' +
          '<button class="imnd-reset" id="imnd-reset" title="关闭全部作弊功能">全部关闭</button>' +
          '<button class="imnd-close" id="imnd-close" title="关闭">×</button>' +
        '</div>' +
        '<div class="imnd-stats">' +
          '<div class="imnd-stat"><b id="imnd-st-money">0</b><span>金币</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-soul">0</b><span>灵魂</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-level">0</b><span>等级</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-hp">0 / 0</b><span>血量</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-day">—</b><span>天数</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-time">—</b><span>本波剩余</span></div>' +
          '<div class="imnd-stat"><b id="imnd-st-enemy">—</b><span>敌人</span></div>' +
        '</div>' +
        '<div class="imnd-scroll">' +
          rowsHtml +
        '</div>' +
        '<div class="imnd-actions-wrap">' +
          '<div class="imnd-sec-title">操作</div>' +
          '<div class="imnd-speed"><label>游戏速度<span class="imnd-desc">影响战斗节奏，1× 为原速</span></label>' +
            '<select id="imnd-speed">' +
              '<option value="0.5">0.5×</option><option value="1" selected>1×</option>' +
              '<option value="1.5">1.5×</option><option value="2">2×</option>' +
              '<option value="3">3×</option><option value="5">5×</option>' +
            '</select></div>' +
          '<div class="imnd-actions">' +
            '<button class="imnd-btn" data-action="kill">清空本波 <kbd>F5</kbd></button>' +
            '<button class="imnd-btn" data-action="endwave">立刻结束本波</button>' +
            '<button class="imnd-btn" data-action="levelup">升一级</button>' +
            '<button class="imnd-btn" data-action="exp">+1,000 经验</button>' +
            '<button class="imnd-btn" data-action="money">+10,000 金币</button>' +
            '<button class="imnd-btn" data-action="soul">+1,000 灵魂</button>' +
            '<button class="imnd-btn imnd-wide" data-action="reload">重新检测游戏模块</button>' +
          '</div>' +
        '</div>' +
        '<div class="imnd-foot">F2 显示/隐藏 · ESC 关闭 · F3/F9 金币/灵魂 · 仅供单机游玩</div>' +
      '</div>' +
      '<div id="imnd-fab">F2 作弊</div>' +
      '<div id="imnd-toast"></div>';

    document.body.appendChild(root);
    bindUI();
    instantiated.ui = true;
    updateStats();
  }

  function bindUI() {
    var panel = document.getElementById('imnd-panel');
    var fab = document.getElementById('imnd-fab');
    var close = document.getElementById('imnd-close');
    var toastEl = document.getElementById('imnd-toast');

    // 面板内的指针事件不要传给游戏
    ['pointerdown', 'mousedown', 'mouseup', 'click', 'wheel', 'contextmenu'].forEach(function (ev) {
      root().addEventListener(ev, function (e) { e.stopPropagation(); }, false);
    });

    // 开关
    root().addEventListener('change', function (e) {
      var t = e.target;
      if (t && t.getAttribute && t.getAttribute('data-toggle')) {
        setToggle(t.getAttribute('data-toggle'), t.checked);
      }
    });

    // 按钮
    root().addEventListener('click', function (e) {
      var btn = e.target.closest('[data-action]');
      if (btn) {
        var act = btn.getAttribute('data-action');
        if (act === 'kill') killWave();
        else if (act === 'endwave') endWave();
        else if (act === 'levelup') levelUp();
        else if (act === 'exp') addExp();
        else if (act === 'money') addMoney();
        else if (act === 'soul') addSoul();
        else if (act === 'reset') resetAll();
        else if (act === 'reload') { instantiated.hooks = false; installHooks(); toast('已重新检测游戏模块'); }
        setTimeout(function () {
          var c = document.getElementById('GameCanvas');
          if (c && c.focus) c.focus();
        }, 0);
      }
    });

    document.getElementById('imnd-reset').addEventListener('click', resetAll);

    document.getElementById('imnd-speed').addEventListener('change', function (e) {
      setSpeed(parseFloat(e.target.value) || 1);
      setTimeout(function () {
        var c = document.getElementById('GameCanvas');
        if (c && c.focus) c.focus();
      }, 0);
    });

    fab.addEventListener('click', function () { togglePanel(true); });
    close.addEventListener('click', function () { togglePanel(false); });
    toastEl.textContent = '';
  }

  function root() { return document.getElementById('imnd-cheat-root'); }

  function togglePanel(force) {
    var panel = document.getElementById('imnd-panel');
    if (!panel) return;
    var open = (typeof force === 'boolean') ? force : !instantiated.panelOpen;
    instantiated.panelOpen = open;
    panel.classList.toggle('imnd-hidden', !open);
    if (open) {
      updateStats();
      syncAllCheckboxes();
    }
    setTimeout(function () {
      var c = document.getElementById('GameCanvas');
      if (!open && c && c.focus) c.focus();
    }, 0);
  }

  function toast(msg) {
    var el = document.getElementById('imnd-toast');
    if (!el) return;
    el.textContent = msg;
    el.classList.add('imnd-show');
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.classList.remove('imnd-show'); }, 1800);
  }

  /* ------------------------------------------------------------------ *
   * 键盘（捕获阶段，优先于游戏自己的监听）
   * ------------------------------------------------------------------ */
  function onKeyDown(e) {
    var k = e.key;
    var fnKey = /^F([2-9])$/.exec(k);

    // 这些键即使重复触发也要拦截，避免 F5 刷新页面等
    if (fnKey) {
      e.preventDefault();
      e.stopImmediatePropagation();
      if (e.repeat) return;
      var nfn = parseInt(fnKey[1], 10);
      if (nfn === 2) togglePanel();
      else if (nfn === 3) setToggle('money', !state.money);
      else if (nfn === 4) setToggle('god', !state.god);
      else if (nfn === 5) killWave();
      else if (nfn === 6) setToggle('autoKill', !state.autoKill);
      else if (nfn === 7) setToggle('freeShop', !state.freeShop);
      else if (nfn === 8) setToggle('infiniteRefresh', !state.infiniteRefresh);
      else if (nfn === 9) setToggle('soul', !state.soul);
      return;
    }

    if (k === 'Escape' && instantiated.panelOpen) {
      e.preventDefault();
      e.stopImmediatePropagation();
      togglePanel(false);
    }
  }

  /* ------------------------------------------------------------------ *
   * 启动
   * ------------------------------------------------------------------ */
  function boot() {
    if (!document.body) {
      setTimeout(boot, 50);
      return;
    }
    buildPanel();
    window.__IMND_CHEAT_API__ = {
      version: VERSION,
      reset: resetAll,
      show: function () { togglePanel(true); },
      hide: function () { togglePanel(false); }
    };
    window.addEventListener('keydown', onKeyDown, true);
    setInterval(tick, 250);
    tick();

    // 等游戏模块就绪后再装 hooks
    var iv = setInterval(function () {
      installHooks();
      if (instantiated.hooks) {
        clearInterval(iv);
        console.log('[IMND-Cheat] ready, press F2');
      }
    }, 500);

    setTimeout(function () {
      toast('作弊 Mod 已加载：按 F2 打开菜单');
    }, 2500);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
