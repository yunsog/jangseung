// 장승 화면 동작 : (1) 표의 줄을 누르면 상세를 펼침 (2) 언어 / 테마를 바꿀 때 입력해 둔 값을 같이 넘김
document.addEventListener("click", function (e) {
  var a = e.target.closest("a.sw");        // 언어 / 테마 링크 : 폼에 써 둔 (아직 안 보낸) 값을 주소에 실음
  if (a) {
    var u = new URL(a.href, location.href);
    document.querySelectorAll("main form [name]").forEach(function (el) {
      if (el.value !== "") u.searchParams.set(el.name, el.value);
    });
    a.href = u.toString();
    return;
  }
  if (e.target.closest("a")) return;       // 링크는 그대로
  var r = e.target.closest("tr.tg");
  if (r) r.nextElementSibling.hidden = !r.nextElementSibling.hidden;
});

// "내부" 메뉴 : 기본은 숨김. 로그인 상태면 /internal/ping 이 204 (로그인 아니면 nginx 가 다른 사이트로 302 -> 받지 않고 숨김 유지).
// 공개 화면은 사람마다 다르지 않게 (캐시 가능) 두고, 로그인 여부는 이 요청 하나로만 앎
(function () {
  var a = document.getElementById("nav-int");
  if (!a || !a.hidden) return;
  fetch(a.href + "ping", { credentials: "same-origin", redirect: "manual", cache: "no-store" })
    .then(function (r) { if (r.ok) a.hidden = false; })
    .catch(function () {});
})();
